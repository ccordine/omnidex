package queue

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresStepAttemptMigrationRejectsRunningLegacyStep(t *testing.T) {
	ctx, pool, migration := openStepAttemptMigrationTest(t)
	tx := beginStepAttemptMigrationSchema(t, ctx, pool, "running")
	defer tx.Rollback(context.Background())
	seedStepAttemptMigrationStep(t, ctx, tx, "running")

	_, err := tx.Exec(ctx, migration)
	if err == nil || !strings.Contains(err.Error(), "running legacy job step") {
		t.Fatalf("migration error=%v, want running legacy step rejection", err)
	}
}

func TestPostgresStepAttemptMigrationRejectsUnresolvedLegacyMutation(t *testing.T) {
	ctx, pool, migration := openStepAttemptMigrationTest(t)
	tx := beginStepAttemptMigrationSchema(t, ctx, pool, "mutation")
	defer tx.Rollback(context.Background())
	jobID, stepID := seedStepAttemptMigrationStep(t, ctx, tx, "pending")
	if _, err := tx.Exec(ctx, `
		INSERT INTO repository_mutation_operations (job_id,generation,step_id,status)
		VALUES ($1,1,$2,'prepared')
	`, jobID, stepID); err != nil {
		t.Fatal(err)
	}

	_, err := tx.Exec(ctx, migration)
	if err == nil || !strings.Contains(err.Error(), "unresolved legacy repository mutation") {
		t.Fatalf("migration error=%v, want unresolved mutation rejection", err)
	}
}

func TestPostgresStepAttemptMigrationCreatesExactLeaseSchema(t *testing.T) {
	ctx, pool, migration := openStepAttemptMigrationTest(t)
	tx := beginStepAttemptMigrationSchema(t, ctx, pool, "schema")
	defer tx.Rollback(context.Background())
	jobID, stepID := seedStepAttemptMigrationStep(t, ctx, tx, "pending")
	if _, err := tx.Exec(ctx, migration); err != nil {
		t.Fatal(err)
	}

	var currentAttempt int64
	if err := tx.QueryRow(ctx, `SELECT current_attempt FROM job_steps WHERE id=$1`, stepID).Scan(&currentAttempt); err != nil {
		t.Fatal(err)
	}
	if currentAttempt != 0 {
		t.Fatalf("current_attempt=%d, want zero before first claim", currentAttempt)
	}

	var leaseSeconds int
	if err := tx.QueryRow(ctx, `
		INSERT INTO job_step_attempts (job_id,generation,step_id,attempt,worker_id)
		VALUES ($1,1,$2,1,'worker-one')
		RETURNING EXTRACT(EPOCH FROM (expires_at-renewed_at))::INT
	`, jobID, stepID).Scan(&leaseSeconds); err != nil {
		t.Fatal(err)
	}
	if leaseSeconds != 75 {
		t.Fatalf("lease seconds=%d, want 75", leaseSeconds)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO job_step_attempts (job_id,generation,step_id,attempt,worker_id)
		VALUES ($1,1,$2,2,'worker-two')
	`, jobID, stepID); err == nil {
		t.Fatal("second active attempt unexpectedly accepted")
	}
}

func TestPostgresStepAttemptMigrationEnforcesMonotonicImmutableHistory(t *testing.T) {
	ctx, pool, migration := openStepAttemptMigrationTest(t)
	tx := beginStepAttemptMigrationSchema(t, ctx, pool, "history")
	defer tx.Rollback(context.Background())
	jobID, stepID := seedStepAttemptMigrationStep(t, ctx, tx, "pending")
	if _, err := tx.Exec(ctx, migration); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO job_step_attempts (job_id,generation,step_id,attempt,worker_id)
		VALUES ($1,1,$2,1,'worker-one')
	`, jobID, stepID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE job_steps SET current_attempt=1 WHERE id=$1`, stepID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE job_step_attempts
		SET status='expired',finished_at=clock_timestamp()
		WHERE job_id=$1 AND generation=1 AND step_id=$2 AND attempt=1
	`, jobID, stepID); err != nil {
		t.Fatal(err)
	}

	expectStepAttemptStatementFailure(t, ctx, tx, `
		INSERT INTO job_step_attempts (job_id,generation,step_id,attempt,worker_id)
		VALUES ($1,1,$2,3,'worker-three')
	`, jobID, stepID)
	if _, err := tx.Exec(ctx, `
		INSERT INTO job_step_attempts (job_id,generation,step_id,attempt,worker_id)
		VALUES ($1,1,$2,2,'worker-two')
	`, jobID, stepID); err != nil {
		t.Fatal(err)
	}
	expectStepAttemptStatementFailure(t, ctx, tx, `
		UPDATE job_step_attempts SET worker_id='worker-three'
		WHERE job_id=$1 AND generation=1 AND step_id=$2 AND attempt=2
	`, jobID, stepID)
	expectStepAttemptStatementFailure(t, ctx, tx, `
		DELETE FROM job_step_attempts
		WHERE job_id=$1 AND generation=1 AND step_id=$2 AND attempt=1
	`, jobID, stepID)
}

func expectStepAttemptStatementFailure(
	t *testing.T,
	ctx context.Context,
	tx pgx.Tx,
	statement string,
	args ...any,
) {
	t.Helper()
	check, err := tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := check.Exec(ctx, statement, args...); err == nil {
		_ = check.Rollback(ctx)
		t.Fatal("invalid step-attempt history mutation unexpectedly succeeded")
	}
	if err := check.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
}

func openStepAttemptMigrationTest(t *testing.T) (context.Context, *pgxpool.Pool, string) {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("set OMNI_TEST_DATABASE_URL to run PostgreSQL step-attempt migration tests")
	}
	migration, err := os.ReadFile("../../migrations/037_step_attempt_leases.sql")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return ctx, pool, string(migration)
}

func beginStepAttemptMigrationSchema(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	suffix string,
) pgx.Tx {
	t.Helper()
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	schema := "step_attempt_" + suffix + "_" + strings.ReplaceAll(time.Now().Format("150405.000000000"), ".", "")
	if _, err := tx.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL search_path TO "+schema+", public"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, stepAttemptMigrationSchema); err != nil {
		t.Fatal(err)
	}
	return tx
}

func seedStepAttemptMigrationStep(
	t *testing.T,
	ctx context.Context,
	tx pgx.Tx,
	status string,
) (int64, int64) {
	t.Helper()
	var jobID, stepID int64
	if err := tx.QueryRow(ctx, `INSERT INTO jobs DEFAULT VALUES RETURNING id`).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO job_generations VALUES ($1,1)`, jobID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO job_steps (job_id,generation,status) VALUES ($1,1,$2) RETURNING id
	`, jobID, status).Scan(&stepID); err != nil {
		t.Fatal(err)
	}
	return jobID, stepID
}

const stepAttemptMigrationSchema = `
CREATE TABLE jobs (id BIGSERIAL PRIMARY KEY);
CREATE TABLE job_generations (
    job_id BIGINT NOT NULL REFERENCES jobs(id), generation BIGINT NOT NULL,
    PRIMARY KEY (job_id,generation)
);
CREATE TABLE job_steps (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL,
    status TEXT NOT NULL,
    worker_id TEXT,
    UNIQUE (job_id,generation,id),
    FOREIGN KEY (job_id,generation) REFERENCES job_generations(job_id,generation)
);
CREATE TABLE repository_mutation_operations (
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL,
    step_id BIGINT NOT NULL,
    status TEXT NOT NULL
);
CREATE TABLE context_projections (
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL,
    step_id BIGINT NOT NULL
);
CREATE TABLE llm_call_evidence (
    job_id BIGINT NOT NULL,
    job_generation BIGINT NOT NULL,
    step_id BIGINT NOT NULL
);
`

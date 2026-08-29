package queue

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresJobGenerationMigrationRejectsAmbiguousLegacyRows(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("set OMNI_TEST_DATABASE_URL to run PostgreSQL job generation preflight tests")
	}
	migration, err := os.ReadFile("../../migrations/028_job_generations.sql")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	tests := []struct {
		name      string
		wantError string
		seed      func(context.Context, pgx.Tx, generationPreflightRows)
	}{
		{
			name: "nonterminal legacy replan", wantError: "nonterminal legacy-replanned job",
			seed: func(ctx context.Context, tx pgx.Tx, rows generationPreflightRows) {
				mustGenerationPreflightExec(t, ctx, tx, `
					INSERT INTO step_contexts (step_id, key) VALUES ($1, 'replan_feedback')
				`, rows.firstStep)
			},
		},
		{
			name: "cross-job artifact", wantError: "cross-job or orphan artifact",
			seed: func(ctx context.Context, tx pgx.Tx, rows generationPreflightRows) {
				mustGenerationPreflightExec(t, ctx, tx, `
					INSERT INTO artifacts (job_id, step_id) VALUES ($1, $2)
				`, rows.firstJob, rows.secondStep)
			},
		},
		{
			name: "missing-step artifact", wantError: "cross-job or orphan artifact",
			seed: func(ctx context.Context, tx pgx.Tx, rows generationPreflightRows) {
				mustGenerationPreflightExec(t, ctx, tx, `
					INSERT INTO artifacts (job_id, step_id) VALUES ($1, 9223372036854775807)
				`, rows.firstJob)
			},
		},
		{
			name: "orphan evidence", wantError: "cross-job or orphan evidence",
			seed: func(ctx context.Context, tx pgx.Tx, rows generationPreflightRows) {
				mustGenerationPreflightExec(t, ctx, tx, `
					INSERT INTO evidence (job_id) VALUES ($1)
				`, rows.firstJob)
			},
		},
		{
			name: "missing-step evidence", wantError: "cross-job or orphan evidence",
			seed: func(ctx context.Context, tx pgx.Tx, rows generationPreflightRows) {
				mustGenerationPreflightExec(t, ctx, tx, `
					INSERT INTO evidence (job_id, step_id) VALUES ($1, 9223372036854775807)
				`, rows.firstJob)
			},
		},
		{
			name: "cross-job llm evidence", wantError: "cross-job LLM call evidence",
			seed: func(ctx context.Context, tx pgx.Tx, rows generationPreflightRows) {
				mustGenerationPreflightExec(t, ctx, tx, `
					INSERT INTO llm_call_evidence (job_id, step_id) VALUES ($1, $2)
				`, rows.firstJob, rows.secondStep)
			},
		},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			schema := fmt.Sprintf("generation_preflight_%d_%d", time.Now().UnixNano(), index)
			mustGenerationPreflightExec(t, ctx, tx, "CREATE SCHEMA "+schema)
			mustGenerationPreflightExec(t, ctx, tx, "SET LOCAL search_path TO "+schema+", public")
			mustGenerationPreflightExec(t, ctx, tx, generationPreflightSchema)
			rows := seedGenerationPreflightJobs(t, ctx, tx)
			test.seed(ctx, tx, rows)
			_, migrationErr := tx.Exec(ctx, string(migration))
			if migrationErr == nil || !strings.Contains(migrationErr.Error(), test.wantError) {
				t.Fatalf("migration error=%v, want %q", migrationErr, test.wantError)
			}
		})
	}
}

type generationPreflightRows struct {
	firstJob, secondJob   int64
	firstStep, secondStep int64
}

func seedGenerationPreflightJobs(t *testing.T, ctx context.Context, tx pgx.Tx) generationPreflightRows {
	t.Helper()
	var rows generationPreflightRows
	mustGenerationPreflightQuery(t, ctx, tx,
		`INSERT INTO jobs (status) VALUES ('running') RETURNING id`, nil, &rows.firstJob)
	mustGenerationPreflightQuery(t, ctx, tx,
		`INSERT INTO jobs (status) VALUES ('running') RETURNING id`, nil, &rows.secondJob)
	mustGenerationPreflightQuery(t, ctx, tx,
		`INSERT INTO job_steps (job_id, action, sort_index) VALUES ($1, 'v3_coding', 0) RETURNING id`,
		[]any{rows.firstJob}, &rows.firstStep)
	mustGenerationPreflightQuery(t, ctx, tx,
		`INSERT INTO job_steps (job_id, action, sort_index) VALUES ($1, 'v3_coding', 0) RETURNING id`,
		[]any{rows.secondJob}, &rows.secondStep)
	return rows
}

func mustGenerationPreflightExec(t *testing.T, ctx context.Context, tx pgx.Tx, query string, args ...any) {
	t.Helper()
	if _, err := tx.Exec(ctx, query, args...); err != nil {
		t.Fatal(err)
	}
}

func mustGenerationPreflightQuery(t *testing.T, ctx context.Context, tx pgx.Tx, query string, args []any, target *int64) {
	t.Helper()
	if err := tx.QueryRow(ctx, query, args...).Scan(target); err != nil {
		t.Fatal(err)
	}
}

const generationPreflightSchema = `
CREATE TABLE jobs (id BIGSERIAL PRIMARY KEY, status TEXT NOT NULL);
CREATE TABLE job_steps (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL REFERENCES jobs(id),
    action TEXT NOT NULL,
    sort_index INTEGER NOT NULL,
    UNIQUE (job_id, id)
);
CREATE TABLE step_contexts (id BIGSERIAL PRIMARY KEY, step_id BIGINT, key TEXT);
CREATE TABLE artifacts (id BIGSERIAL PRIMARY KEY, job_id BIGINT, step_id BIGINT);
CREATE TABLE evidence (id BIGSERIAL PRIMARY KEY, job_id BIGINT, step_id BIGINT);
CREATE TABLE claims (
    id BIGSERIAL PRIMARY KEY, job_id BIGINT, step_id BIGINT,
    status TEXT NOT NULL DEFAULT 'unsupported', confidence DOUBLE PRECISION NOT NULL DEFAULT 0
);
CREATE TABLE llm_call_evidence (id BIGSERIAL PRIMARY KEY, job_id BIGINT, step_id BIGINT);
CREATE TABLE claim_support (
    id BIGSERIAL PRIMARY KEY,
    claim_id BIGINT NOT NULL REFERENCES claims(id),
    evidence_id BIGINT NOT NULL REFERENCES evidence(id),
    support_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    rationale TEXT
);
CREATE TABLE memory_candidates (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT,
    status TEXT NOT NULL DEFAULT 'candidate',
    confidence DOUBLE PRECISION NOT NULL DEFAULT 0
);
CREATE TABLE task_events (id BIGSERIAL PRIMARY KEY, job_id BIGINT NOT NULL);
CREATE FUNCTION task_ledger_text_is_exact(value TEXT) RETURNS BOOLEAN AS $$
    SELECT value <> '' AND value = btrim(value);
$$ LANGUAGE SQL IMMUTABLE STRICT;
`

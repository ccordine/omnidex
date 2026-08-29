package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestPostgresJobGenerationBoundaryIsExactAndJobOwned(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "067")); err != nil {
		t.Fatal(err)
	}

	seedTx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer seedTx.Rollback(context.Background())
	firstJob := insertGenerationTestJob(t, ctx, seedTx, "first")
	secondJob := insertGenerationTestJob(t, ctx, seedTx, "second")
	firstStep := insertGenerationTestStep(t, ctx, seedTx, firstJob, 1, "v3_coding")
	secondStep := insertGenerationTestStep(t, ctx, seedTx, secondJob, 1, "v3_coding")
	historicalLLMID := insertHistoricalGenerationLLMEvidence(t, ctx, seedTx, firstJob, firstStep)
	if err := seedTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "180")); err != nil {
		t.Fatal(err)
	}
	memoryScope := createMemoryScopeForTest(t, repository)

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())

	feedback := "Use the accepted user correction."
	feedbackSHA := generationTestSHA256(feedback)
	if _, err := tx.Exec(ctx, `
		INSERT INTO job_generations (
			job_id, generation, purpose, predecessor_generation,
			boundary_action, feedback, feedback_sha256
		) VALUES ($1, 2, 'replan', 1, 'v3_coding', $2, $3)
	`, firstJob, feedback, feedbackSHA); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE jobs SET current_generation=2 WHERE id=$1`, firstJob); err != nil {
		t.Fatal(err)
	}
	secondGenerationStep := insertGenerationTestStep(t, ctx, tx, firstJob, 2, "v3_coding")
	if _, err := tx.Exec(ctx, `
		UPDATE job_steps SET superseded_at_generation=2 WHERE id=$1
	`, firstStep); err != nil {
		t.Fatal(err)
	}

	expectGenerationDatabaseFailure(t, ctx, tx, `
		INSERT INTO job_generations (job_id, generation, purpose)
		VALUES ($1, 2, 'initial')
	`, secondJob)
	expectGenerationDatabaseFailure(t, ctx, tx, `
		INSERT INTO job_generations (
			job_id, generation, purpose, predecessor_generation,
			boundary_action, feedback, feedback_sha256
		) VALUES ($1, 3, 'replan', 1, 'v3_coding', $2, $3)
	`, firstJob, feedback, feedbackSHA)
	expectGenerationDatabaseFailure(t, ctx, tx, `
		INSERT INTO job_generations (
			job_id, generation, purpose, predecessor_generation,
			boundary_action, feedback, feedback_sha256
		) VALUES ($1, 3, 'replan', 2, 'unregistered', $2, $3)
	`, firstJob, feedback, feedbackSHA)
	expectGenerationDatabaseFailure(t, ctx, tx, `
		UPDATE job_generations SET feedback='Changed', feedback_sha256=$2
		WHERE job_id=$1 AND generation=2
	`, firstJob, generationTestSHA256("Changed"))
	expectGenerationDatabaseFailure(t, ctx, tx, `
		UPDATE jobs SET current_generation=1 WHERE id=$1
	`, firstJob)
	expectGenerationDatabaseFailure(t, ctx, tx, `
		UPDATE job_steps SET generation=1 WHERE id=$1
	`, secondGenerationStep)
	activateStepAttemptInTxForTest(
		t, ctx, tx, firstJob, 2, secondGenerationStep,
		testStepAttemptWorker("generation-boundary", secondGenerationStep),
	)
	expectGenerationDatabaseFailure(t, ctx, tx, `
		UPDATE job_steps SET status='pending' WHERE id=$1
	`, secondGenerationStep)
	expectGenerationDatabaseFailure(t, ctx, tx, `
		UPDATE job_steps SET superseded_at_generation=NULL WHERE id=$1
	`, firstStep)
	expectGenerationDatabaseFailure(t, ctx, tx, `
		DELETE FROM job_steps WHERE id=$1
	`, firstStep)
	expectGenerationDatabaseFailure(t, ctx, tx, `
		INSERT INTO job_steps (job_id, generation, action, sort_index, status)
		VALUES ($1, 2, 'v3_coding', 0, 'pending')
	`, secondJob)

	assertGenerationDerivedOwnership(
		t, ctx, tx, firstJob, secondJob, secondStep, historicalLLMID,
	)
	assertGenerationMemoryCandidateOwnership(
		t, ctx, tx, firstJob, secondJob, memoryScope.ProjectID, string(memoryScope.ChannelID),
	)
}

func insertHistoricalGenerationLLMEvidence(
	t *testing.T,
	ctx context.Context,
	tx pgx.Tx,
	jobID, stepID int64,
) int64 {
	t.Helper()
	var llmID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO llm_call_evidence (
			job_id, job_generation, step_id, scope, requested_model, model, attempt,
			system_prompt, user_prompt, request_sha256, response_format,
			context_tokens, max_output_tokens, status, error, latency_ms
		) VALUES (
			$1, 1, $2, 'generation-test', 'requested', 'effective', 1,
			'system', 'user', $3, 'text', 1024, 128,
			'preparation_failed', 'expected historical failure', 1
		) RETURNING id
	`, jobID, stepID, strings.Repeat("a", 64)).Scan(&llmID); err != nil {
		t.Fatal(err)
	}
	return llmID
}

func insertGenerationTestJob(t *testing.T, ctx context.Context, tx pgx.Tx, suffix string) int64 {
	t.Helper()
	var jobID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO jobs (instruction, pipeline, status, metadata)
		VALUES ($1, 'chat', 'pending', '{}'::jsonb)
		RETURNING id
	`, fmt.Sprintf("generation-%s-%d", suffix, time.Now().UnixNano())).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO job_generations (job_id, generation, purpose)
		VALUES ($1, 1, 'initial')
	`, jobID); err != nil {
		t.Fatal(err)
	}
	return jobID
}

func insertGenerationTestStep(t *testing.T, ctx context.Context, tx pgx.Tx, jobID, generation int64, action string) int64 {
	t.Helper()
	var stepID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO job_steps (job_id, generation, action, sort_index, status)
		VALUES ($1, $2, $3, 0, 'pending')
		RETURNING id
	`, jobID, generation, action).Scan(&stepID); err != nil {
		t.Fatal(err)
	}
	return stepID
}

func expectGenerationDatabaseFailure(t *testing.T, ctx context.Context, tx pgx.Tx, statement string, arguments ...any) {
	t.Helper()
	const savepoint = "generation_expected_failure"
	if _, err := tx.Exec(ctx, "SAVEPOINT "+savepoint); err != nil {
		t.Fatal(err)
	}
	_, operationErr := tx.Exec(ctx, statement, arguments...)
	if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT "+savepoint); err != nil {
		t.Fatalf("recover expected PostgreSQL failure: %v (operation error: %v)", err, operationErr)
	}
	if _, err := tx.Exec(ctx, "RELEASE SAVEPOINT "+savepoint); err != nil {
		t.Fatal(err)
	}
	if operationErr == nil {
		t.Fatalf("PostgreSQL accepted forbidden generation statement: %s", strings.TrimSpace(statement))
	}
}

func generationTestSHA256(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

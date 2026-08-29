package queue

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

func assertGenerationDerivedOwnership(
	t *testing.T,
	ctx context.Context,
	tx pgx.Tx,
	firstJob, secondJob, secondStep, llmID int64,
) {
	t.Helper()
	if _, err := tx.Exec(ctx, `
		INSERT INTO artifacts (kind, version, payload_json)
		VALUES ('global', '1', '{}'::jsonb)
	`); err != nil {
		t.Fatalf("insert global artifact: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO evidence (kind, payload_json)
		VALUES ('global', '{}'::jsonb)
	`); err != nil {
		t.Fatalf("insert global evidence: %v", err)
	}
	expectGenerationDatabaseFailure(t, ctx, tx, `
		INSERT INTO artifacts (job_id, step_id, kind, version, payload_json)
		VALUES ($1, $2, 'cross-job', '1', '{}'::jsonb)
	`, firstJob, secondStep)
	expectGenerationDatabaseFailure(t, ctx, tx, `
		INSERT INTO artifacts (job_id, kind, version, payload_json)
		VALUES ($1, 'orphan', '1', '{}'::jsonb)
	`, firstJob)
	expectGenerationDatabaseFailure(t, ctx, tx, `
		INSERT INTO evidence (job_id, step_id, kind, payload_json)
		VALUES ($1, $2, 'cross-job', '{}'::jsonb)
	`, firstJob, secondStep)

	if llmID < 1 {
		t.Fatal("historical LLM evidence fixture is missing")
	}
	expectGenerationDatabaseFailure(t, ctx, tx, `
		UPDATE llm_call_evidence SET job_id=$1 WHERE id=$2
	`, secondJob, llmID)
}

func assertGenerationMemoryCandidateOwnership(
	t *testing.T,
	ctx context.Context,
	tx pgx.Tx,
	firstJob, secondJob, projectID int64,
	channelID string,
) {
	t.Helper()
	if _, err := tx.Exec(ctx, `
		INSERT INTO memory_candidates (
			project_id, channel_id, job_id, generation, candidate_kind, content
		) VALUES ($1, $2, $3, 2, 'episodic', 'current candidate')
	`, projectID, channelID, firstJob); err != nil {
		t.Fatalf("insert generation-bound memory candidate: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO memory_candidates (project_id, channel_id, candidate_kind, content)
		VALUES ($1, $2, 'scope', 'scope candidate')
	`, projectID, channelID); err != nil {
		t.Fatalf("insert scope-bound memory candidate: %v", err)
	}
	expectGenerationDatabaseFailure(t, ctx, tx, `
		INSERT INTO memory_candidates (
			project_id, channel_id, job_id, candidate_kind, content
		) VALUES ($1, $2, $3, 'missing-generation', 'invalid')
	`, projectID, channelID, firstJob)
	expectGenerationDatabaseFailure(t, ctx, tx, `
		INSERT INTO memory_candidates (
			project_id, channel_id, job_id, generation, candidate_kind, content
		) VALUES ($1, $2, $3, 2, 'unknown-generation', 'invalid')
	`, projectID, channelID, secondJob)
	expectGenerationDatabaseFailure(t, ctx, tx, `
		INSERT INTO memory_candidates (
			project_id, channel_id, generation, candidate_kind, content
		) VALUES ($1, $2, 1, 'orphan-generation', 'invalid')
	`, projectID, channelID)
	expectGenerationDatabaseFailure(t, ctx, tx, `
		INSERT INTO memory_candidates (
			project_id, channel_id, candidate_kind, content, status
		) VALUES ($1, $2, 'episodic', 'invalid status', 'unknown')
	`, projectID, channelID)
	expectGenerationDatabaseFailure(t, ctx, tx, `
		INSERT INTO memory_candidates (
			project_id, channel_id, candidate_kind, content, confidence
		) VALUES ($1, $2, 'episodic', 'invalid confidence', 2)
	`, projectID, channelID)
}

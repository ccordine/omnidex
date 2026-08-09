package queue

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func assertGenerationDerivedOwnership(
	t *testing.T,
	ctx context.Context,
	tx pgx.Tx,
	firstJob, secondJob, firstStep, secondStep int64,
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

	var evidenceID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO evidence (job_id, step_id, kind, payload_json)
		VALUES ($1, $2, 'verification', '{}'::jsonb)
		RETURNING id
	`, firstJob, firstStep).Scan(&evidenceID); err != nil {
		t.Fatal(err)
	}
	var firstClaimID, secondClaimID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO claims (job_id, step_id, text, normalized_text)
		VALUES ($1, $2, 'first claim', 'first claim')
		RETURNING id
	`, firstJob, firstStep).Scan(&firstClaimID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO claims (job_id, step_id, text, normalized_text)
		VALUES ($1, $2, 'second claim', 'second claim')
		RETURNING id
	`, secondJob, secondStep).Scan(&secondClaimID); err != nil {
		t.Fatal(err)
	}
	expectGenerationDatabaseFailure(t, ctx, tx, `
		INSERT INTO claims (job_id, step_id, text, normalized_text)
		VALUES ($1, $2, 'cross job', 'cross job')
	`, firstJob, secondStep)
	expectGenerationDatabaseFailure(t, ctx, tx, `
		INSERT INTO claims (job_id, step_id, text, normalized_text, status)
		VALUES ($1, $2, 'bad status', 'bad status', 'unknown')
	`, firstJob, firstStep)
	expectGenerationDatabaseFailure(t, ctx, tx, `
		INSERT INTO claims (job_id, step_id, text, normalized_text, confidence)
		VALUES ($1, $2, 'bad confidence', 'bad confidence', 2)
	`, firstJob, firstStep)
	if _, err := tx.Exec(ctx, `
		INSERT INTO claim_support (job_id, claim_id, evidence_id, support_score, rationale)
		VALUES ($1, $2, $3, 1, 'Exact support evidence.')
	`, firstJob, firstClaimID, evidenceID); err != nil {
		t.Fatalf("insert same-job claim support: %v", err)
	}
	expectGenerationDatabaseFailure(t, ctx, tx, `
		INSERT INTO claim_support (job_id, claim_id, evidence_id, support_score, rationale)
		VALUES ($1, $2, $3, 1, 'Cross-job evidence.')
	`, firstJob, secondClaimID, evidenceID)
	expectGenerationDatabaseFailure(t, ctx, tx, `
		INSERT INTO claim_support (job_id, claim_id, evidence_id, support_score, rationale)
		VALUES ($1, $2, $3, 2, 'Invalid score.')
	`, firstJob, firstClaimID, evidenceID)

	var llmID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO llm_call_evidence (
			job_id, job_generation, step_id, scope, requested_model, model, attempt,
			system_prompt, user_prompt, request_sha256, response_format,
			context_tokens, max_output_tokens, status, error, latency_ms
		) VALUES (
			$1, 1, $2, 'generation-test', 'requested', 'effective', 1,
			'system', 'user', $3, 'text', 1024, 128,
			'preparation_failed', 'expected failure', 1
		) RETURNING id
	`, firstJob, firstStep, strings.Repeat("a", 64)).Scan(&llmID); err != nil {
		t.Fatal(err)
	}
	expectGenerationDatabaseFailure(t, ctx, tx, `
		UPDATE llm_call_evidence SET job_id=$1 WHERE id=$2
	`, secondJob, llmID)
}

func assertGenerationMemoryCandidateOwnership(
	t *testing.T,
	ctx context.Context,
	tx pgx.Tx,
	firstJob, secondJob int64,
) {
	t.Helper()
	if _, err := tx.Exec(ctx, `
		INSERT INTO memory_candidates (
			job_id, generation, candidate_kind, content
		) VALUES ($1, 2, 'episodic', 'current candidate')
	`, firstJob); err != nil {
		t.Fatalf("insert generation-bound memory candidate: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO memory_candidates (candidate_kind, content)
		VALUES ('global', 'global candidate')
	`); err != nil {
		t.Fatalf("insert global memory candidate: %v", err)
	}
	expectGenerationDatabaseFailure(t, ctx, tx, `
		INSERT INTO memory_candidates (job_id, candidate_kind, content)
		VALUES ($1, 'missing-generation', 'invalid')
	`, firstJob)
	expectGenerationDatabaseFailure(t, ctx, tx, `
		INSERT INTO memory_candidates (
			job_id, generation, candidate_kind, content
		) VALUES ($1, 2, 'unknown-generation', 'invalid')
	`, secondJob)
	expectGenerationDatabaseFailure(t, ctx, tx, `
		INSERT INTO memory_candidates (
			generation, candidate_kind, content
		) VALUES (1, 'orphan-generation', 'invalid')
	`)
	expectGenerationDatabaseFailure(t, ctx, tx, `
		INSERT INTO memory_candidates (candidate_kind, content, status)
		VALUES ('episodic', 'invalid status', 'unknown')
	`)
	expectGenerationDatabaseFailure(t, ctx, tx, `
		INSERT INTO memory_candidates (candidate_kind, content, confidence)
		VALUES ('episodic', 'invalid confidence', 2)
	`)
}

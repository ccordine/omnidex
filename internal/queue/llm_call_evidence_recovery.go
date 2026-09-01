package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

// LatestReusableLLMCallEvidence returns the newest immutable call for one
// deterministic portable work ID under the current step. A reclaimed attempt
// may observe only evidence owned by an expired predecessor attempt; canceled,
// failed, superseded, and unrelated step evidence is never reusable.
func (r *Repository) LatestReusableLLMCallEvidence(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	workID string,
) (LLMCallEvidence, bool, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return LLMCallEvidence{}, false, fmt.Errorf(
			"reusable LLM call lookup requires context and PostgreSQL",
		)
	}
	if !exactLowerSHA256(workID) {
		return LLMCallEvidence{}, false, fmt.Errorf(
			"reusable LLM call lookup requires one exact work ID",
		)
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return LLMCallEvidence{}, false, fmt.Errorf(
			"begin reusable LLM call lookup: %w", err,
		)
	}
	defer tx.Rollback(ctx)
	if _, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, authority); err != nil {
		return LLMCallEvidence{}, false, err
	} else if stepStatus != model.StepStatusRunning {
		return LLMCallEvidence{}, false, staleStepAttemptError(
			authority, "reusable LLM call step is not running", nil,
		)
	}
	var evidence LLMCallEvidence
	err = scanLLMCallEvidenceWithOutcome(tx.QueryRow(ctx, `SELECT `+llmCallEvidenceColumns+`,
		outcomes.call_evidence_id,outcomes.status,outcomes.candidate_sha256,
		outcomes.projection,outcomes.validation_error,outcomes.validation_error_sha256,
		outcomes.created_at
		FROM llm_call_evidence AS calls
		JOIN job_step_attempts AS attempts
		  ON attempts.job_id=calls.job_id AND attempts.generation=calls.generation
		 AND attempts.step_id=calls.step_id AND attempts.attempt=calls.step_attempt
		 AND attempts.worker_id=calls.worker_id
		LEFT JOIN llm_call_receipts AS receipts ON receipts.call_evidence_id=calls.id
		LEFT JOIN llm_call_outcomes AS outcomes ON outcomes.call_evidence_id=calls.id
		WHERE calls.job_id=$1 AND calls.generation=$2 AND calls.step_id=$3
		  AND calls.work_id=$4
		  AND (
		      (calls.step_attempt=$5 AND calls.worker_id=$6)
		      OR
		      (calls.step_attempt<$5 AND attempts.status='expired')
		  )
		ORDER BY calls.step_attempt DESC,calls.iteration DESC,calls.id DESC
		LIMIT 1`,
		authority.JobID, authority.Generation, authority.StepID, workID,
		authority.Attempt, authority.WorkerID,
	), &evidence)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.Commit(ctx); err != nil {
			return LLMCallEvidence{}, false, fmt.Errorf(
				"commit empty reusable LLM call lookup: %w", err,
			)
		}
		return LLMCallEvidence{}, false, nil
	}
	if err != nil {
		return LLMCallEvidence{}, false, fmt.Errorf(
			"read reusable LLM call evidence: %w", err,
		)
	}
	if err := tx.Commit(ctx); err != nil {
		return LLMCallEvidence{}, false, fmt.Errorf(
			"commit reusable LLM call lookup: %w", err,
		)
	}
	return evidence, true, nil
}

// ReusableLLMCallChildEvidence returns the immutable next call in one source
// correction lineage. The parent-child key is unique in PostgreSQL, so replay
// cannot choose between competing continuations.
func (r *Repository) ReusableLLMCallChildEvidence(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	parentCallID int64,
) (LLMCallEvidence, bool, error) {
	if ctx == nil || r == nil || r.pool == nil || parentCallID < 1 {
		return LLMCallEvidence{}, false, fmt.Errorf(
			"reusable LLM child lookup requires context, PostgreSQL, and one parent call",
		)
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return LLMCallEvidence{}, false, fmt.Errorf(
			"begin reusable LLM child lookup: %w", err,
		)
	}
	defer tx.Rollback(ctx)
	if _, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, authority); err != nil {
		return LLMCallEvidence{}, false, err
	} else if stepStatus != model.StepStatusRunning {
		return LLMCallEvidence{}, false, staleStepAttemptError(
			authority, "reusable LLM child step is not running", nil,
		)
	}
	var evidence LLMCallEvidence
	err = scanLLMCallEvidenceWithOutcome(tx.QueryRow(ctx, `SELECT `+llmCallEvidenceColumns+`,
		outcomes.call_evidence_id,outcomes.status,outcomes.candidate_sha256,
		outcomes.projection,outcomes.validation_error,outcomes.validation_error_sha256,
		outcomes.created_at
		FROM llm_call_evidence AS calls
		JOIN job_step_attempts AS attempts
		  ON attempts.job_id=calls.job_id AND attempts.generation=calls.generation
		 AND attempts.step_id=calls.step_id AND attempts.attempt=calls.step_attempt
		 AND attempts.worker_id=calls.worker_id
		LEFT JOIN llm_call_receipts AS receipts ON receipts.call_evidence_id=calls.id
		LEFT JOIN llm_call_outcomes AS outcomes ON outcomes.call_evidence_id=calls.id
		WHERE calls.parent_call_evidence_id=$1
		  AND calls.job_id=$2 AND calls.generation=$3 AND calls.step_id=$4
		  AND (
		      (calls.step_attempt=$5 AND calls.worker_id=$6)
		      OR
		      (calls.step_attempt<$5 AND attempts.status='expired')
		  )
		LIMIT 1`,
		parentCallID, authority.JobID, authority.Generation, authority.StepID,
		authority.Attempt, authority.WorkerID,
	), &evidence)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.Commit(ctx); err != nil {
			return LLMCallEvidence{}, false, fmt.Errorf(
				"commit empty reusable LLM child lookup: %w", err,
			)
		}
		return LLMCallEvidence{}, false, nil
	}
	if err != nil {
		return LLMCallEvidence{}, false, fmt.Errorf(
			"read reusable LLM child evidence: %w", err,
		)
	}
	if err := tx.Commit(ctx); err != nil {
		return LLMCallEvidence{}, false, fmt.Errorf(
			"commit reusable LLM child lookup: %w", err,
		)
	}
	return evidence, true, nil
}

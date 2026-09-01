package queue

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (r *Repository) FinalizeLLMCallEvidence(
	ctx context.Context,
	record LLMCallReceiptRecord,
) (LLMCallEvidence, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return LLMCallEvidence{}, fmt.Errorf("LLM call receipt requires context and PostgreSQL")
	}
	normalized, err := normalizeLLMCallReceipt(record)
	if err != nil {
		return LLMCallEvidence{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return LLMCallEvidence{}, fmt.Errorf("begin exact LLM receipt write: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := bindLLMCallReceiptOpening(ctx, tx, normalized); err != nil {
		return LLMCallEvidence{}, err
	}
	if err := insertLLMCallReceiptTx(ctx, tx, normalized); err != nil {
		return LLMCallEvidence{}, err
	}
	outcome, found, err := readLLMCallOutcomeTx(ctx, tx, record.CallEvidenceID)
	if err != nil {
		return LLMCallEvidence{}, err
	}
	if !found && normalized.status == LLMCallFailed {
		outcome, err = insertLLMCallOutcomeTx(
			ctx, tx, record.CallEvidenceID, LLMCallProviderFailed,
			normalized.candidateSHA256, nil, record.CallError,
		)
		found = err == nil
	}
	if err != nil {
		return LLMCallEvidence{}, err
	}
	evidence, err := readLLMCallEvidenceByIDTx(ctx, tx, record.CallEvidenceID)
	if err != nil {
		return LLMCallEvidence{}, err
	}
	if found {
		evidence.Outcome = &outcome
	}
	if err := tx.Commit(ctx); err != nil {
		return LLMCallEvidence{}, fmt.Errorf("commit exact LLM receipt: %w", err)
	}
	return evidence, nil
}

func bindLLMCallReceiptOpening(
	ctx context.Context,
	tx pgx.Tx,
	normalized normalizedLLMCallReceipt,
) error {
	record := normalized.record
	var attemptStatus, modelInputSHA256, providerRequestSHA256 string
	var attemptUnexpired bool
	err := tx.QueryRow(ctx, `
		SELECT attempts.status,attempts.expires_at>clock_timestamp(),
		       calls.model_input_sha256,calls.provider_request_sha256
		FROM llm_call_evidence AS calls
		JOIN job_step_attempts AS attempts
		  ON attempts.job_id=calls.job_id AND attempts.generation=calls.generation
		 AND attempts.step_id=calls.step_id AND attempts.attempt=calls.step_attempt
		 AND attempts.worker_id=calls.worker_id
		WHERE calls.id=$1 AND calls.job_id=$2 AND calls.generation=$3
		  AND calls.step_id=$4 AND calls.step_attempt=$5 AND calls.worker_id=$6
		FOR UPDATE OF attempts
	`, record.CallEvidenceID, record.Authority.JobID, record.Authority.Generation,
		record.Authority.StepID, record.Authority.Attempt, record.Authority.WorkerID,
	).Scan(&attemptStatus, &attemptUnexpired, &modelInputSHA256, &providerRequestSHA256)
	if err != nil {
		return fmt.Errorf("bind exact LLM receipt opening: %w", err)
	}
	if attemptStatus != "active" || !attemptUnexpired {
		return fmt.Errorf(
			"%w: call %d origin attempt status %q unexpired=%t",
			ErrLLMCallTerminalizedByAttempt, record.CallEvidenceID,
			attemptStatus, attemptUnexpired,
		)
	}
	if modelInputSHA256 != normalized.modelInputSHA256 ||
		providerRequestSHA256 != normalized.providerRequestSHA256 {
		return fmt.Errorf("LLM call receipt request differs from its immutable opening")
	}
	return nil
}

func insertLLMCallReceiptTx(
	ctx context.Context,
	tx pgx.Tx,
	normalized normalizedLLMCallReceipt,
) error {
	record := normalized.record
	rawResponse, rawResponseSHA256 := optionalLLMCallRawResponse(normalized)
	candidate, candidateSHA256 := optionalLLMCallCandidate(normalized)
	callError := optionalExactText(record.CallError)
	callErrorSHA256 := optionalLLMCallEvidenceHash(record.CallError)
	result, err := tx.Exec(ctx, `
		INSERT INTO llm_call_receipts (
			call_evidence_id,generation_receipt,generation_receipt_sha256,
			raw_response_present,raw_response,raw_response_sha256,raw_response_bytes,
			candidate,candidate_sha256,prompt_tokens,output_tokens,
			provider_duration_nanos,output_limit_reached,status,error,error_sha256,elapsed_nanos
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17
		)
	`, record.CallEvidenceID, normalized.generationReceipt,
		normalized.generationReceiptSHA256, normalized.rawResponsePresent,
		rawResponse, rawResponseSHA256, len(normalized.rawResponse),
		candidate, candidateSHA256, record.Generation.Usage.PromptEvalCount,
		record.Generation.Usage.EvalCount, record.Generation.Usage.TotalDurationNanos,
		record.OutputLimitReached, string(normalized.status), callError, callErrorSHA256,
		record.Elapsed.Nanoseconds())
	if err != nil {
		return fmt.Errorf("record exact LLM provider receipt: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("exact LLM provider receipt was not recorded")
	}
	return nil
}

func optionalLLMCallRawResponse(normalized normalizedLLMCallReceipt) (any, any) {
	if !normalized.rawResponsePresent {
		return nil, nil
	}
	value := make([]byte, len(normalized.rawResponse))
	copy(value, normalized.rawResponse)
	return value, normalized.rawResponseSHA256
}

func optionalLLMCallCandidate(normalized normalizedLLMCallReceipt) (any, any) {
	if normalized.record.Generation.Content == "" {
		return nil, nil
	}
	return normalized.record.Generation.Content, normalized.candidateSHA256
}

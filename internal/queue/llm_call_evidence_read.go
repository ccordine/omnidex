package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

const llmCallEvidenceColumns = `
	calls.id,calls.job_id,calls.generation,calls.step_id,calls.step_attempt,calls.worker_id,
	 calls.scope,calls.work_id,calls.work_kind,calls.iteration,calls.output_continuation,
	 calls.dispatch_attempt,calls.parent_call_evidence_id,calls.replaces_call_evidence_id,
	calls.source_base_candidate,calls.source_base_sha256,calls.source_start_byte,
	calls.source_end_byte,
	calls.source_question,calls.source_question_sha256,
	calls.requested_model,calls.model,calls.protocol,
	calls.system_envelope,calls.model_input,calls.model_input_sha256,
	calls.model_input_bytes,calls.provider_request,calls.provider_request_sha256,
	calls.provider_request_bytes,calls.context_tokens,calls.max_output_tokens,
	calls.output_limit_mode,calls.created_at,
	receipts.call_evidence_id,receipts.generation_receipt,receipts.generation_receipt_sha256,
	receipts.raw_response_present,receipts.raw_response,receipts.raw_response_sha256,
	receipts.raw_response_bytes,receipts.candidate,receipts.candidate_sha256,
	 receipts.prompt_tokens,receipts.output_tokens,receipts.provider_duration_nanos,
	 receipts.output_limit_reached,
	receipts.status,receipts.error,receipts.error_sha256,receipts.elapsed_nanos,
	receipts.created_at`

func insertLLMCallOutcomeTx(
	ctx context.Context,
	tx pgx.Tx,
	callID int64,
	status LLMCallOutcomeStatus,
	candidateSHA256 string,
	projection []byte,
	validationError string,
) (LLMCallOutcome, error) {
	projectionValue := any(nil)
	if len(projection) > 0 {
		projectionValue = string(projection)
	}
	errorValue := any(nil)
	if validationError != "" {
		errorValue = validationError
	}
	candidateValue := any(nil)
	if candidateSHA256 != "" {
		candidateValue = candidateSHA256
	}
	var outcome LLMCallOutcome
	var rawProjection []byte
	var rawCandidateSHA256, rawError, rawErrorSHA256 *string
	if err := tx.QueryRow(ctx, `
		INSERT INTO llm_call_outcomes (
			call_evidence_id,status,candidate_sha256,projection,
			validation_error,validation_error_sha256
		) VALUES ($1,$2,$3,$4::jsonb,$5,$6)
		RETURNING call_evidence_id,status,candidate_sha256,projection,
		          validation_error,validation_error_sha256,created_at
	`, callID, string(status), candidateValue, projectionValue, errorValue,
		optionalLLMCallEvidenceHash(validationError)).Scan(
		&outcome.CallEvidenceID, &outcome.Status, &rawCandidateSHA256,
		&rawProjection, &rawError, &rawErrorSHA256, &outcome.CreatedAt,
	); err != nil {
		return LLMCallOutcome{}, fmt.Errorf("record exact LLM call outcome: %w", err)
	}
	outcome.Projection = append(json.RawMessage(nil), rawProjection...)
	if rawCandidateSHA256 != nil {
		outcome.CandidateSHA256 = *rawCandidateSHA256
	}
	if rawError != nil {
		outcome.ValidationError = *rawError
	}
	if rawErrorSHA256 != nil {
		outcome.ValidationErrorSHA256 = *rawErrorSHA256
	}
	return outcome, nil
}

func readLLMCallOutcomeTx(
	ctx context.Context,
	tx pgx.Tx,
	callID int64,
) (LLMCallOutcome, bool, error) {
	var outcome LLMCallOutcome
	var rawProjection []byte
	var candidateSHA256, validationError, validationErrorSHA256 *string
	err := tx.QueryRow(ctx, `
		SELECT call_evidence_id,status,candidate_sha256,projection,
		       validation_error,validation_error_sha256,created_at
		FROM llm_call_outcomes WHERE call_evidence_id=$1
	`, callID).Scan(
		&outcome.CallEvidenceID, &outcome.Status, &candidateSHA256,
		&rawProjection, &validationError, &validationErrorSHA256, &outcome.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return LLMCallOutcome{}, false, nil
	}
	if err != nil {
		return LLMCallOutcome{}, false, fmt.Errorf("read exact LLM call outcome: %w", err)
	}
	if candidateSHA256 != nil {
		outcome.CandidateSHA256 = *candidateSHA256
	}
	outcome.Projection = append(json.RawMessage(nil), rawProjection...)
	if validationError != nil {
		outcome.ValidationError = *validationError
	}
	if validationErrorSHA256 != nil {
		outcome.ValidationErrorSHA256 = *validationErrorSHA256
	}
	return outcome, true, nil
}

func readLLMCallEvidenceByIDTx(
	ctx context.Context,
	tx pgx.Tx,
	callID int64,
) (LLMCallEvidence, error) {
	var evidence LLMCallEvidence
	err := scanLLMCallEvidenceWithOutcome(tx.QueryRow(ctx, `SELECT `+llmCallEvidenceColumns+`,
		outcomes.call_evidence_id,outcomes.status,outcomes.candidate_sha256,
		outcomes.projection,outcomes.validation_error,outcomes.validation_error_sha256,
		outcomes.created_at
		FROM llm_call_evidence AS calls
		LEFT JOIN llm_call_receipts AS receipts ON receipts.call_evidence_id=calls.id
		LEFT JOIN llm_call_outcomes AS outcomes ON outcomes.call_evidence_id=calls.id
		WHERE calls.id=$1
	`, callID), &evidence)
	if err != nil {
		return LLMCallEvidence{}, fmt.Errorf("read exact LLM call evidence: %w", err)
	}
	return evidence, nil
}

func (r *Repository) GetLLMCallEvidence(
	ctx context.Context,
	callID int64,
) (LLMCallEvidence, error) {
	if ctx == nil || r == nil || r.pool == nil || callID < 1 {
		return LLMCallEvidence{}, fmt.Errorf(
			"exact LLM call evidence lookup requires context, PostgreSQL, and a positive call",
		)
	}
	var evidence LLMCallEvidence
	if err := scanLLMCallEvidenceWithOutcome(r.pool.QueryRow(
		ctx,
		`SELECT `+llmCallEvidenceColumns+`,
		 outcomes.call_evidence_id,outcomes.status,outcomes.candidate_sha256,
		 outcomes.projection,outcomes.validation_error,outcomes.validation_error_sha256,
		 outcomes.created_at
		 FROM llm_call_evidence AS calls
		 LEFT JOIN llm_call_receipts AS receipts ON receipts.call_evidence_id=calls.id
		 LEFT JOIN llm_call_outcomes AS outcomes ON outcomes.call_evidence_id=calls.id
		 WHERE calls.id=$1`,
		callID,
	), &evidence); err != nil {
		return LLMCallEvidence{}, fmt.Errorf("read exact LLM call evidence: %w", err)
	}
	return evidence, nil
}

func (r *Repository) ListLLMCallEvidenceForJob(
	ctx context.Context,
	jobID, afterID int64,
	limit int,
) ([]LLMCallEvidence, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return nil, fmt.Errorf("LLM call evidence history requires context and PostgreSQL")
	}
	if jobID < 1 || afterID < 0 || limit < 1 || limit > MaxLLMCallEvidencePageSize {
		return nil, fmt.Errorf("LLM call evidence history requires a positive job, nonnegative cursor, and limit between 1 and %d", MaxLLMCallEvidencePageSize)
	}
	rows, err := r.pool.Query(ctx, `SELECT `+llmCallEvidenceColumns+`,
		outcomes.call_evidence_id,outcomes.status,outcomes.candidate_sha256,
		outcomes.projection,outcomes.validation_error,outcomes.validation_error_sha256,
		outcomes.created_at
		FROM llm_call_evidence AS calls
		LEFT JOIN llm_call_receipts AS receipts ON receipts.call_evidence_id=calls.id
		LEFT JOIN llm_call_outcomes AS outcomes ON outcomes.call_evidence_id=calls.id
		WHERE calls.job_id=$1 AND calls.id>$2
		ORDER BY calls.id ASC
		LIMIT $3
	`, jobID, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("list exact LLM call evidence: %w", err)
	}
	defer rows.Close()
	items := make([]LLMCallEvidence, 0, limit)
	for rows.Next() {
		var item LLMCallEvidence
		if err := scanLLMCallEvidenceWithOutcome(rows, &item); err != nil {
			return nil, fmt.Errorf("scan exact LLM call evidence: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate exact LLM call evidence: %w", err)
	}
	return items, nil
}

type llmCallEvidenceScanner interface {
	Scan(dest ...any) error
}

func scanLLMCallOpening(scanner llmCallEvidenceScanner, evidence *LLMCallEvidence) error {
	var providerRequest []byte
	var parentCallEvidenceID, replacesCallEvidenceID *int64
	var sourceBaseCandidate, sourceBaseSHA256, sourceQuestion, sourceQuestionSHA256 *string
	var sourceStartByte, sourceEndByte *int
	if err := scanner.Scan(
		&evidence.ID, &evidence.JobID, &evidence.Generation, &evidence.StepID,
		&evidence.StepAttempt, &evidence.WorkerID, &evidence.Scope, &evidence.WorkID,
		&evidence.WorkKind, &evidence.Iteration, &evidence.OutputContinuation,
		&evidence.DispatchAttempt, &parentCallEvidenceID, &replacesCallEvidenceID,
		&sourceBaseCandidate, &sourceBaseSHA256, &sourceStartByte, &sourceEndByte,
		&sourceQuestion, &sourceQuestionSHA256,
		&evidence.RequestedModel, &evidence.Model, &evidence.Protocol,
		&evidence.SystemEnvelope, &evidence.ModelInput,
		&evidence.ModelInputSHA256, &evidence.ModelInputBytes, &providerRequest,
		&evidence.ProviderRequestSHA256, &evidence.ProviderRequestBytes,
		&evidence.ContextTokens, &evidence.MaxOutputTokens, &evidence.OutputLimitMode,
		&evidence.CreatedAt,
	); err != nil {
		return err
	}
	if parentCallEvidenceID != nil {
		evidence.ParentCallEvidenceID = *parentCallEvidenceID
	}
	if replacesCallEvidenceID != nil {
		evidence.ReplacesCallEvidenceID = *replacesCallEvidenceID
	}
	assignLLMCallSourceCorrection(
		evidence, sourceBaseCandidate, sourceBaseSHA256, sourceStartByte,
		sourceEndByte, sourceQuestion, sourceQuestionSHA256,
	)
	evidence.ProviderRequest = append([]byte(nil), providerRequest...)
	return nil
}

func scanLLMCallEvidenceWithOutcome(
	scanner llmCallEvidenceScanner,
	evidence *LLMCallEvidence,
) error {
	var rawResponse, providerRequest, generationReceipt []byte
	var rawResponseSHA256, candidate, candidateSHA256, callError, callErrorSHA256 *string
	var receiptID *int64
	var generationReceiptSHA256, receiptStatus *string
	var rawResponsePresent, outputLimitReached *bool
	var rawResponseBytes, promptTokens, outputTokens *int
	var providerDurationNanos, elapsedNanos *int64
	var receiptCreatedAt *time.Time
	var outcomeID *int64
	var outcomeStatus, outcomeCandidateSHA256, outcomeError, outcomeErrorSHA256 *string
	var outcomeProjection []byte
	var outcomeCreatedAt *time.Time
	var parentCallEvidenceID, replacesCallEvidenceID *int64
	var sourceBaseCandidate, sourceBaseSHA256, sourceQuestion, sourceQuestionSHA256 *string
	var sourceStartByte, sourceEndByte *int
	if err := scanner.Scan(
		&evidence.ID, &evidence.JobID, &evidence.Generation, &evidence.StepID,
		&evidence.StepAttempt, &evidence.WorkerID, &evidence.Scope, &evidence.WorkID,
		&evidence.WorkKind, &evidence.Iteration, &evidence.OutputContinuation,
		&evidence.DispatchAttempt, &parentCallEvidenceID, &replacesCallEvidenceID,
		&sourceBaseCandidate, &sourceBaseSHA256, &sourceStartByte, &sourceEndByte,
		&sourceQuestion, &sourceQuestionSHA256,
		&evidence.RequestedModel, &evidence.Model, &evidence.Protocol,
		&evidence.SystemEnvelope, &evidence.ModelInput,
		&evidence.ModelInputSHA256, &evidence.ModelInputBytes, &providerRequest,
		&evidence.ProviderRequestSHA256, &evidence.ProviderRequestBytes,
		&evidence.ContextTokens, &evidence.MaxOutputTokens, &evidence.OutputLimitMode,
		&evidence.CreatedAt, &receiptID, &generationReceipt, &generationReceiptSHA256,
		&rawResponsePresent, &rawResponse, &rawResponseSHA256, &rawResponseBytes,
		&candidate, &candidateSHA256, &promptTokens, &outputTokens,
		&providerDurationNanos, &outputLimitReached, &receiptStatus, &callError, &callErrorSHA256,
		&elapsedNanos, &receiptCreatedAt,
		&outcomeID, &outcomeStatus, &outcomeCandidateSHA256, &outcomeProjection,
		&outcomeError, &outcomeErrorSHA256, &outcomeCreatedAt,
	); err != nil {
		return err
	}
	if parentCallEvidenceID != nil {
		evidence.ParentCallEvidenceID = *parentCallEvidenceID
	}
	if replacesCallEvidenceID != nil {
		evidence.ReplacesCallEvidenceID = *replacesCallEvidenceID
	}
	assignLLMCallSourceCorrection(
		evidence, sourceBaseCandidate, sourceBaseSHA256, sourceStartByte,
		sourceEndByte, sourceQuestion, sourceQuestionSHA256,
	)
	evidence.ProviderRequest = append([]byte(nil), providerRequest...)
	if receiptID != nil {
		evidence.ProviderReceiptPresent = true
		evidence.GenerationReceipt = append(json.RawMessage(nil), generationReceipt...)
		evidence.GenerationReceiptSHA256 = *generationReceiptSHA256
		evidence.RawResponsePresent = *rawResponsePresent
		if *rawResponsePresent {
			evidence.RawResponse = make([]byte, len(rawResponse))
			copy(evidence.RawResponse, rawResponse)
		}
		evidence.RawResponseBytes = *rawResponseBytes
		evidence.PromptTokens = *promptTokens
		evidence.OutputTokens = *outputTokens
		evidence.ProviderDurationNanos = *providerDurationNanos
		evidence.OutputLimitReached = *outputLimitReached
		evidence.Status = LLMCallStatus(*receiptStatus)
		evidence.ElapsedNanos = *elapsedNanos
		evidence.ProviderReceiptCreatedAt = receiptCreatedAt
		if rawResponseSHA256 != nil {
			evidence.RawResponseSHA256 = *rawResponseSHA256
		}
		if candidate != nil {
			evidence.Candidate = *candidate
		}
		if candidateSHA256 != nil {
			evidence.CandidateSHA256 = *candidateSHA256
		}
		if callError != nil {
			evidence.Error = *callError
		}
		if callErrorSHA256 != nil {
			evidence.ErrorSHA256 = *callErrorSHA256
		}
	}
	if outcomeID != nil {
		outcome := &LLMCallOutcome{CallEvidenceID: *outcomeID}
		outcome.Status = LLMCallOutcomeStatus(*outcomeStatus)
		if outcomeCandidateSHA256 != nil {
			outcome.CandidateSHA256 = *outcomeCandidateSHA256
		}
		outcome.Projection = append(json.RawMessage(nil), outcomeProjection...)
		if outcomeError != nil {
			outcome.ValidationError = *outcomeError
		}
		if outcomeErrorSHA256 != nil {
			outcome.ValidationErrorSHA256 = *outcomeErrorSHA256
		}
		if outcomeCreatedAt != nil {
			outcome.CreatedAt = *outcomeCreatedAt
		}
		evidence.Outcome = outcome
	}
	return nil
}

func assignLLMCallSourceCorrection(
	evidence *LLMCallEvidence,
	baseCandidate *string,
	baseSHA256 *string,
	startByte *int,
	endByte *int,
	question *string,
	questionSHA256 *string,
) {
	if evidence == nil || baseCandidate == nil {
		return
	}
	evidence.SourceBaseCandidate = *baseCandidate
	if baseSHA256 != nil {
		evidence.SourceBaseSHA256 = *baseSHA256
	}
	if startByte != nil {
		evidence.SourceStartByte = *startByte
	}
	if endByte != nil {
		evidence.SourceEndByte = *endByte
	}
	if question != nil {
		evidence.SourceQuestion = *question
	}
	if questionSHA256 != nil {
		evidence.SourceQuestionSHA256 = *questionSHA256
	}
}

func optionalLLMCallEvidenceHash(value string) any {
	if value == "" {
		return nil
	}
	return llmEvidenceSHA256([]byte(value))
}

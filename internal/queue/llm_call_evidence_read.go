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
	 calls.scope,calls.work_input,calls.work_kind,calls.iteration,calls.output_continuation,
	 calls.dispatch_attempt,calls.parent_call_evidence_id,calls.replaces_call_evidence_id,
	calls.source_base_candidate,calls.source_start_byte,
	calls.source_end_byte,
	calls.source_question,
	calls.requested_model,calls.model,calls.protocol,
	calls.model_input,
	calls.model_input_bytes,calls.provider_request,
	calls.provider_request_bytes,calls.context_tokens,calls.max_output_tokens,
	calls.output_limit_mode,calls.created_at,
	receipts.call_evidence_id,receipts.generation_receipt,
	receipts.raw_response_present,receipts.raw_response,
	receipts.raw_response_bytes,receipts.candidate,
	 receipts.prompt_tokens,receipts.output_tokens,receipts.provider_duration_nanos,
	 receipts.output_limit_reached,
	receipts.status,receipts.error,receipts.elapsed_nanos,
	receipts.created_at`

func readLLMCallEvidenceByIDTx(
	ctx context.Context,
	tx pgx.Tx,
	callID int64,
) (LLMCallEvidence, error) {
	var evidence LLMCallEvidence
	err := scanLLMCallEvidenceWithOutcome(tx.QueryRow(ctx, `SELECT `+llmCallEvidenceColumns+`,
		outcomes.call_evidence_id,outcomes.status,
		outcomes.validation_error,
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
		 outcomes.call_evidence_id,outcomes.status,
		 outcomes.validation_error,
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
		outcomes.call_evidence_id,outcomes.status,
		outcomes.validation_error,
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
	var sourceBaseCandidate, sourceQuestion *string
	var sourceStartByte, sourceEndByte *int
	if err := scanner.Scan(
		&evidence.ID, &evidence.JobID, &evidence.Generation, &evidence.StepID,
		&evidence.StepAttempt, &evidence.WorkerID, &evidence.Scope, &evidence.WorkInput,
		&evidence.WorkKind, &evidence.Iteration, &evidence.OutputContinuation,
		&evidence.DispatchAttempt, &parentCallEvidenceID, &replacesCallEvidenceID,
		&sourceBaseCandidate, &sourceStartByte, &sourceEndByte,
		&sourceQuestion,
		&evidence.RequestedModel, &evidence.Model, &evidence.Protocol,
		&evidence.ModelInput,
		&evidence.ModelInputBytes, &providerRequest,
		&evidence.ProviderRequestBytes,
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
		evidence, sourceBaseCandidate, sourceStartByte,
		sourceEndByte, sourceQuestion,
	)
	evidence.ProviderRequest = append([]byte(nil), providerRequest...)
	return nil
}

func scanLLMCallEvidenceWithOutcome(
	scanner llmCallEvidenceScanner,
	evidence *LLMCallEvidence,
) error {
	var rawResponse, providerRequest, generationReceipt []byte
	var candidate, callError *string
	var receiptID *int64
	var receiptStatus *string
	var rawResponsePresent, outputLimitReached *bool
	var rawResponseBytes, promptTokens, outputTokens *int
	var providerDurationNanos, elapsedNanos *int64
	var receiptCreatedAt *time.Time
	var outcomeID *int64
	var outcomeStatus, outcomeError *string
	var outcomeCreatedAt *time.Time
	var parentCallEvidenceID, replacesCallEvidenceID *int64
	var sourceBaseCandidate, sourceQuestion *string
	var sourceStartByte, sourceEndByte *int
	if err := scanner.Scan(
		&evidence.ID, &evidence.JobID, &evidence.Generation, &evidence.StepID,
		&evidence.StepAttempt, &evidence.WorkerID, &evidence.Scope, &evidence.WorkInput,
		&evidence.WorkKind, &evidence.Iteration, &evidence.OutputContinuation,
		&evidence.DispatchAttempt, &parentCallEvidenceID, &replacesCallEvidenceID,
		&sourceBaseCandidate, &sourceStartByte, &sourceEndByte,
		&sourceQuestion,
		&evidence.RequestedModel, &evidence.Model, &evidence.Protocol,
		&evidence.ModelInput,
		&evidence.ModelInputBytes, &providerRequest,
		&evidence.ProviderRequestBytes,
		&evidence.ContextTokens, &evidence.MaxOutputTokens, &evidence.OutputLimitMode,
		&evidence.CreatedAt, &receiptID, &generationReceipt,
		&rawResponsePresent, &rawResponse, &rawResponseBytes,
		&candidate, &promptTokens, &outputTokens,
		&providerDurationNanos, &outputLimitReached, &receiptStatus, &callError,
		&elapsedNanos, &receiptCreatedAt,
		&outcomeID, &outcomeStatus,
		&outcomeError, &outcomeCreatedAt,
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
		evidence, sourceBaseCandidate, sourceStartByte,
		sourceEndByte, sourceQuestion,
	)
	evidence.ProviderRequest = append([]byte(nil), providerRequest...)
	if receiptID != nil {
		evidence.ProviderReceiptPresent = true
		evidence.GenerationReceipt = append(json.RawMessage(nil), generationReceipt...)
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
		if candidate != nil {
			evidence.Candidate = *candidate
		}
		if callError != nil {
			evidence.Error = *callError
		}
	}
	if outcomeID != nil {
		outcome := &LLMCallOutcome{CallEvidenceID: *outcomeID}
		outcome.Status = LLMCallOutcomeStatus(*outcomeStatus)
		if outcomeError != nil {
			outcome.ValidationError = *outcomeError
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
	startByte *int,
	endByte *int,
	question *string,
) {
	if evidence == nil || baseCandidate == nil {
		return
	}
	evidence.SourceBaseCandidate = *baseCandidate
	if startByte != nil {
		evidence.SourceStartByte = *startByte
	}
	if endByte != nil {
		evidence.SourceEndByte = *endByte
	}
	if question != nil {
		evidence.SourceQuestion = *question
	}
}

package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ReserveLLMCallEvidence(
	ctx context.Context,
	record LLMCallOpeningRecord,
) (LLMCallEvidence, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return LLMCallEvidence{}, fmt.Errorf("LLM call opening requires context and PostgreSQL")
	}
	normalized, err := normalizeLLMCallOpening(record)
	if err != nil {
		return LLMCallEvidence{}, err
	}
	return insertLLMCallOpening(ctx, r.pool, normalized)
}

func insertLLMCallOpening(
	ctx context.Context,
	querier llmCallEvidenceQuerier,
	normalized normalizedLLMCallOpening,
) (LLMCallEvidence, error) {
	record := normalized.record
	var evidence LLMCallEvidence
	var sourceBaseCandidate, sourceStartByte, sourceEndByte, sourceQuestion any
	if record.SourceCorrection != nil {
		sourceBaseCandidate = record.SourceCorrection.BaseCandidate
		sourceStartByte = record.SourceCorrection.StartByte
		sourceEndByte = record.SourceCorrection.EndByte
		sourceQuestion = record.SourceCorrection.Question
	}
	err := scanLLMCallOpening(querier.QueryRow(ctx, `
		INSERT INTO llm_call_evidence (
			job_id,generation,step_id,step_attempt,worker_id,
			scope,work_input,work_kind,iteration,output_continuation,dispatch_attempt,
			parent_call_evidence_id,replaces_call_evidence_id,
			source_base_candidate,source_start_byte,source_end_byte,source_question,
			requested_model,model,protocol,
			model_input,model_input_bytes,
			provider_request,provider_request_bytes,
			context_tokens,max_output_tokens,output_limit_mode
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,
			$18,$19,$20,$21,$22,$23,$24,$25,$26,$27
		)
		RETURNING id,job_id,generation,step_id,step_attempt,worker_id,
		          scope,work_input,work_kind,iteration,output_continuation,dispatch_attempt,
		          parent_call_evidence_id,replaces_call_evidence_id,
		          source_base_candidate,source_start_byte,source_end_byte,source_question,
		          requested_model,model,protocol,
		          model_input,model_input_bytes,
		          provider_request,provider_request_bytes,
		          context_tokens,max_output_tokens,output_limit_mode,created_at
		`, record.Authority.JobID, record.Authority.Generation, record.Authority.StepID,
		record.Authority.Attempt, record.Authority.WorkerID,
		record.Scope, record.WorkInput, string(record.WorkKind), record.Iteration,
		record.OutputContinuation, record.DispatchAttempt,
		optionalLLMCallParentID(record.ParentCallEvidenceID),
		optionalLLMCallParentID(record.ReplacesCallEvidenceID),
		sourceBaseCandidate, sourceStartByte, sourceEndByte, sourceQuestion, record.RequestedModel,
		record.Prepared.ContextModel, string(record.Prepared.Protocol),
		normalized.modelInput,
		len(normalized.modelInput), normalized.providerRequest,
		len(normalized.providerRequest), record.Prepared.ContextTokens,
		record.Prepared.MaxOutputTokens, string(record.Prepared.OutputLimitMode)), &evidence)
	if err != nil {
		return LLMCallEvidence{}, fmt.Errorf("reserve exact LLM call evidence: %w", err)
	}
	return evidence, nil
}

func optionalLLMCallParentID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

type llmCallEvidenceQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (r *Repository) RecordLLMCallOutcome(
	ctx context.Context,
	record LLMCallOutcomeRecord,
) (LLMCallOutcome, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return LLMCallOutcome{}, fmt.Errorf("LLM call outcome requires context and PostgreSQL")
	}
	if err := validateStepAttemptAuthority(record.Authority); err != nil {
		return LLMCallOutcome{}, err
	}
	if record.CallEvidenceID < 1 || record.Candidate == "" {
		return LLMCallOutcome{}, fmt.Errorf("LLM call outcome requires exact call and candidate identities")
	}
	if record.ValidationError != "" {
		if err := validateLLMCallError(record.ValidationError); err != nil {
			return LLMCallOutcome{}, err
		}
	}
	status := LLMCallAccepted
	if record.ValidationError != "" {
		status = LLMCallRejected
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return LLMCallOutcome{}, fmt.Errorf("begin exact LLM outcome write: %w", err)
	}
	defer tx.Rollback(ctx)
	locked, err := lockStepAttemptAuthorityTx(ctx, tx, record.Authority)
	if err != nil {
		return LLMCallOutcome{}, err
	}
	if err := requireLockedStepAttemptActiveTx(ctx, tx, record.Authority, locked); err != nil {
		return LLMCallOutcome{}, fmt.Errorf(
			"%w: call %d cannot consume a provider receipt: %v",
			ErrLLMCallTerminalizedByAttempt, record.CallEvidenceID, err,
		)
	}
	if locked.StepStatus != model.StepStatusRunning || !jobAcceptsStepTerminal(locked.JobStatus) {
		return LLMCallOutcome{}, staleStepAttemptError(
			record.Authority,
			fmt.Sprintf(
				"LLM outcome writer job status %q step status %q",
				locked.JobStatus, locked.StepStatus,
			),
			nil,
		)
	}
	var candidate string
	if err := tx.QueryRow(ctx, `
		SELECT receipts.candidate
		FROM llm_call_evidence AS calls
		JOIN llm_call_receipts AS receipts ON receipts.call_evidence_id=calls.id
		JOIN job_step_attempts AS origin_attempt
		  ON origin_attempt.job_id=calls.job_id
		 AND origin_attempt.generation=calls.generation
		 AND origin_attempt.step_id=calls.step_id
		 AND origin_attempt.attempt=calls.step_attempt
		 AND origin_attempt.worker_id=calls.worker_id
		WHERE calls.id=$1 AND calls.job_id=$2 AND calls.generation=$3
		  AND calls.step_id=$4
		  AND receipts.status='succeeded'
		  AND (
		      (calls.step_attempt=$5 AND calls.worker_id=$6
		       AND origin_attempt.status='active')
		      OR
		      (calls.step_attempt<$5 AND origin_attempt.status='expired')
		  )
		FOR SHARE OF calls,receipts,origin_attempt
	`, record.CallEvidenceID, record.Authority.JobID, record.Authority.Generation,
		record.Authority.StepID, record.Authority.Attempt, record.Authority.WorkerID,
	).Scan(&candidate); err != nil {
		return LLMCallOutcome{}, fmt.Errorf(
			"bind LLM outcome to exact current or expired-predecessor successful call: %w",
			err,
		)
	}
	if candidate != record.Candidate {
		return LLMCallOutcome{}, fmt.Errorf("LLM call outcome candidate differs from provider evidence")
	}
	outcome, err := insertLLMCallOutcomeTx(
		ctx, tx, record.CallEvidenceID, status, record.ValidationError,
	)
	if err != nil {
		return LLMCallOutcome{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return LLMCallOutcome{}, fmt.Errorf("commit exact LLM call outcome: %w", err)
	}
	return outcome, nil
}

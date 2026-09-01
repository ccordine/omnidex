package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
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
	var sourceBaseCandidate, sourceBaseSHA256, sourceStartByte, sourceEndByte any
	var sourceQuestion, sourceQuestionSHA256 any
	if record.SourceCorrection != nil {
		sourceBaseCandidate = record.SourceCorrection.BaseCandidate
		sourceBaseSHA256 = record.SourceCorrection.BaseSHA256
		sourceStartByte = record.SourceCorrection.StartByte
		sourceEndByte = record.SourceCorrection.EndByte
		sourceQuestion = record.SourceCorrection.Question
		sourceQuestionSHA256 = record.SourceCorrection.QuestionSHA256
	}
	err := scanLLMCallOpening(querier.QueryRow(ctx, `
		INSERT INTO llm_call_evidence (
			job_id,generation,step_id,step_attempt,worker_id,
			scope,work_id,work_kind,iteration,output_continuation,parent_call_evidence_id,
			source_base_candidate,source_base_sha256,source_start_byte,source_end_byte,
			source_question,source_question_sha256,
			requested_model,model,protocol,
			system_envelope,model_input,model_input_sha256,model_input_bytes,
			provider_request,provider_request_sha256,provider_request_bytes,
			context_tokens,max_output_tokens,output_limit_mode
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,
			$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30
		)
		RETURNING id,job_id,generation,step_id,step_attempt,worker_id,
		          scope,work_id,work_kind,iteration,output_continuation,parent_call_evidence_id,
		          source_base_candidate,source_base_sha256,source_start_byte,source_end_byte,
		          source_question,source_question_sha256,
		          requested_model,model,protocol,
		          system_envelope,model_input,model_input_sha256,model_input_bytes,
		          provider_request,provider_request_sha256,provider_request_bytes,
		          context_tokens,max_output_tokens,output_limit_mode,created_at
		`, record.Authority.JobID, record.Authority.Generation, record.Authority.StepID,
		record.Authority.Attempt, record.Authority.WorkerID,
		record.Scope, record.WorkID, string(record.WorkKind), record.Iteration,
		record.OutputContinuation,
		optionalLLMCallParentID(record.ParentCallEvidenceID),
		sourceBaseCandidate, sourceBaseSHA256, sourceStartByte, sourceEndByte,
		sourceQuestion, sourceQuestionSHA256, record.RequestedModel,
		record.Prepared.ContextModel, string(record.Prepared.Protocol),
		record.Prepared.Prompt, normalized.modelInput,
		normalized.modelInputSHA256, len(normalized.modelInput),
		normalized.providerRequest, normalized.providerRequestSHA256,
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
	projectionJSON, err := encodeLLMCallProjection(record.Candidate, record.Projection)
	if err != nil {
		return LLMCallOutcome{}, err
	}
	status := LLMCallAccepted
	if record.ValidationError != "" {
		status = LLMCallRejected
	}
	if status == LLMCallAccepted && len(projectionJSON) == 0 {
		return LLMCallOutcome{}, fmt.Errorf("accepted LLM call outcome requires one exact projection")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return LLMCallOutcome{}, fmt.Errorf("begin exact LLM outcome write: %w", err)
	}
	defer tx.Rollback(ctx)
	var attemptStatus string
	if err := tx.QueryRow(ctx, `
		SELECT status FROM job_step_attempts
		WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4 AND worker_id=$5
		FOR UPDATE
	`, record.Authority.JobID, record.Authority.Generation, record.Authority.StepID,
		record.Authority.Attempt, record.Authority.WorkerID).Scan(&attemptStatus); err != nil {
		return LLMCallOutcome{}, fmt.Errorf("lock exact LLM outcome attempt authority: %w", err)
	}
	var candidateSHA256 string
	if err := tx.QueryRow(ctx, `
		SELECT receipts.candidate_sha256
		FROM llm_call_evidence AS calls
		JOIN llm_call_receipts AS receipts ON receipts.call_evidence_id=calls.id
		WHERE calls.id=$1 AND calls.job_id=$2 AND calls.generation=$3
		  AND calls.step_id=$4 AND calls.step_attempt=$5 AND calls.worker_id=$6
		  AND receipts.status='succeeded'
		FOR SHARE OF calls,receipts
	`, record.CallEvidenceID, record.Authority.JobID, record.Authority.Generation,
		record.Authority.StepID, record.Authority.Attempt, record.Authority.WorkerID,
	).Scan(&candidateSHA256); err != nil {
		return LLMCallOutcome{}, fmt.Errorf("bind LLM outcome to exact successful call: %w", err)
	}
	if candidateSHA256 != llmEvidenceSHA256([]byte(record.Candidate)) {
		return LLMCallOutcome{}, fmt.Errorf("LLM call outcome candidate differs from provider evidence")
	}
	if attemptStatus != "active" {
		outcome, found, err := readLLMCallOutcomeTx(ctx, tx, record.CallEvidenceID)
		if err != nil {
			return LLMCallOutcome{}, err
		}
		if !found {
			return LLMCallOutcome{}, fmt.Errorf(
				"terminal step attempt %q left LLM call %d without an outcome",
				attemptStatus, record.CallEvidenceID,
			)
		}
		return outcome, fmt.Errorf(
			"%w: call %d attempt status %q",
			ErrLLMCallTerminalizedByAttempt,
			record.CallEvidenceID, attemptStatus,
		)
	}

	outcome, err := insertLLMCallOutcomeTx(
		ctx, tx, record.CallEvidenceID, status, candidateSHA256,
		projectionJSON, record.ValidationError,
	)
	if err != nil {
		return LLMCallOutcome{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return LLMCallOutcome{}, fmt.Errorf("commit exact LLM call outcome: %w", err)
	}
	return outcome, nil
}

func encodeLLMCallProjection(
	candidate string,
	projection *assemblyline.PortableResultProjection,
) ([]byte, error) {
	if projection == nil {
		return nil, nil
	}
	if err := projection.ValidateFor(candidate); err != nil {
		return nil, fmt.Errorf("LLM call outcome projection is invalid: %w", err)
	}
	evidence := LLMCallProjectionEvidence{
		Kind:                 projection.Kind,
		SourceResponseSHA256: projection.SourceResponseSHA256,
		SourceSHA256:         projection.SourceSHA256,
		StartByte:            projection.StartByte,
		EndByte:              projection.EndByte,
		RawBytes:             projection.RawBytes,
		DiscardedBytes:       projection.DiscardedBytes,
	}
	raw, err := json.Marshal(evidence)
	if err != nil {
		return nil, fmt.Errorf("encode exact LLM outcome projection: %w", err)
	}
	return raw, nil
}

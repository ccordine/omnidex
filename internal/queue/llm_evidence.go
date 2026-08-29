package queue

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const (
	llmCallEvidenceSelectColumns = `calls.id, calls.job_id, calls.job_generation,
		calls.step_id, calls.step_attempt, calls.worker_id,
		calls.scope, calls.work_id, calls.work_kind, calls.context_projection_id,
		calls.requested_model, calls.model, calls.attempt, calls.system_prompt,
		calls.user_prompt, openings.wire_request_sha256,
		calls.context_tokens, calls.max_output_tokens,
		calls.response, calls.response_sha256, calls.status,
		calls.error, calls.latency_ms, calls.created_at, calls.station_call_opening_id`
)

type LLMCallEvidenceStatus string

const (
	LLMEvidenceGenerationFailed LLMCallEvidenceStatus = "generation_failed"
	LLMEvidenceSucceeded        LLMCallEvidenceStatus = "succeeded"
)

type LLMCallEvidenceRecord struct {
	Authority            model.StepAttemptAuthority
	StepID               int64
	Scope                string
	WorkID               string
	WorkKind             string
	StationCallOpeningID int64
	ContextProjectionID  string
	RequestedModel       string
	Model                string
	Attempt              int
	SystemPrompt         string
	UserPrompt           string
	ContextTokens        int
	MaxOutputTokens      int
	Response             string
	Status               LLMCallEvidenceStatus
	Error                string
	LatencyMS            int64
}

type LLMCallEvidence struct {
	ID                   int64                 `json:"id"`
	JobID                int64                 `json:"job_id"`
	JobGeneration        int64                 `json:"job_generation"`
	StepID               int64                 `json:"step_id"`
	StepAttempt          int64                 `json:"step_attempt"`
	WorkerID             string                `json:"worker_id"`
	Scope                string                `json:"scope"`
	WorkID               string                `json:"work_id,omitempty"`
	WorkKind             string                `json:"work_kind,omitempty"`
	StationCallOpeningID int64                 `json:"station_call_opening_id,omitempty"`
	ContextProjectionID  string                `json:"context_projection_id,omitempty"`
	RequestedModel       string                `json:"requested_model"`
	Model                string                `json:"model"`
	Attempt              int                   `json:"attempt"`
	SystemPrompt         string                `json:"system_prompt"`
	UserPrompt           string                `json:"user_prompt"`
	WireRequestSHA256    string                `json:"wire_request_sha256"`
	ContextTokens        int                   `json:"context_tokens"`
	MaxOutputTokens      int                   `json:"max_output_tokens"`
	Response             string                `json:"response,omitempty"`
	ResponseSHA256       string                `json:"response_sha256,omitempty"`
	Status               LLMCallEvidenceStatus `json:"status"`
	Error                string                `json:"error,omitempty"`
	LatencyMS            int64                 `json:"latency_ms"`
	CreatedAt            time.Time             `json:"created_at"`
}

func insertLLMCallEvidenceTx(
	ctx context.Context,
	tx pgx.Tx,
	record LLMCallEvidenceRecord,
) (LLMCallEvidence, error) {
	record = normalizeLLMCallEvidenceRecord(record)
	if err := validateLLMCallEvidenceRecord(record); err != nil {
		return LLMCallEvidence{}, err
	}
	if record.StepID != record.Authority.StepID {
		return LLMCallEvidence{}, fmt.Errorf("%w: LLM evidence step disagrees with attempt", ErrStaleStepAttempt)
	}
	response, responseHash := optionalLLMResponse(record.Response)
	var evidence LLMCallEvidence
	err := scanLLMCallEvidence(tx.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO llm_call_evidence (
				job_id, job_generation, step_id, scope, work_id, work_kind, context_projection_id,
				requested_model, model, attempt, system_prompt, user_prompt,
				context_tokens, max_output_tokens, response, response_sha256, status, error, latency_ms,
				step_attempt, worker_id, station_call_opening_id
			)
			VALUES ($18,$19,$1,$2,NULLIF($3,''),NULLIF($4,''),NULLIF($5,''),$6,$7,$8,
			        $9,$10,$11,$12,$13,$14,$15,NULLIF($16,''),$17,$20,$21,$22)
			RETURNING id, job_id, job_generation, step_id, step_attempt, worker_id,
			          scope, work_id, work_kind, context_projection_id, requested_model, model,
			          attempt, system_prompt, user_prompt, context_tokens, max_output_tokens,
			          response, response_sha256, status, error, latency_ms, created_at,
			          station_call_opening_id
		)
		SELECT inserted.id, inserted.job_id, inserted.job_generation, inserted.step_id,
		       inserted.step_attempt, inserted.worker_id, inserted.scope, inserted.work_id,
		       inserted.work_kind, inserted.context_projection_id, inserted.requested_model,
		       inserted.model, inserted.attempt, inserted.system_prompt, inserted.user_prompt,
		       openings.wire_request_sha256, inserted.context_tokens, inserted.max_output_tokens,
		       inserted.response, inserted.response_sha256, inserted.status, inserted.error,
		       inserted.latency_ms, inserted.created_at, inserted.station_call_opening_id
		FROM inserted
		JOIN station_call_openings AS openings
		  ON openings.id=inserted.station_call_opening_id
	`, record.StepID, record.Scope, record.WorkID, record.WorkKind, record.ContextProjectionID, record.RequestedModel,
		record.Model, record.Attempt, record.SystemPrompt, record.UserPrompt,
		record.ContextTokens, record.MaxOutputTokens,
		response, responseHash, string(record.Status), record.Error,
		record.LatencyMS, record.Authority.JobID, record.Authority.Generation,
		record.Authority.Attempt, record.Authority.WorkerID,
		record.StationCallOpeningID), &evidence)
	if err != nil {
		return LLMCallEvidence{}, fmt.Errorf("record exact LLM call evidence: %w", err)
	}
	return evidence, nil
}

type llmEvidenceScanner interface {
	Scan(dest ...any) error
}

func scanLLMCallEvidence(scanner llmEvidenceScanner, evidence *LLMCallEvidence) error {
	var workID, workKind, projectionID, response, responseHash, errorText *string
	var stationCallOpeningID *int64
	if err := scanner.Scan(
		&evidence.ID, &evidence.JobID, &evidence.JobGeneration, &evidence.StepID,
		&evidence.StepAttempt, &evidence.WorkerID,
		&evidence.Scope, &workID, &workKind, &projectionID,
		&evidence.RequestedModel, &evidence.Model, &evidence.Attempt, &evidence.SystemPrompt,
		&evidence.UserPrompt, &evidence.WireRequestSHA256,
		&evidence.ContextTokens, &evidence.MaxOutputTokens, &response, &responseHash,
		&evidence.Status, &errorText, &evidence.LatencyMS, &evidence.CreatedAt,
		&stationCallOpeningID,
	); err != nil {
		return err
	}
	evidence.WorkID = llmEvidenceString(workID)
	evidence.WorkKind = llmEvidenceString(workKind)
	evidence.ContextProjectionID = llmEvidenceString(projectionID)
	evidence.Response = llmEvidenceString(response)
	evidence.ResponseSHA256 = llmEvidenceString(responseHash)
	evidence.Error = llmEvidenceString(errorText)
	if stationCallOpeningID != nil {
		evidence.StationCallOpeningID = *stationCallOpeningID
	}
	return nil
}

func normalizeLLMCallEvidenceRecord(record LLMCallEvidenceRecord) LLMCallEvidenceRecord {
	record.Scope = strings.TrimSpace(record.Scope)
	record.WorkID = strings.TrimSpace(record.WorkID)
	record.WorkKind = strings.TrimSpace(record.WorkKind)
	record.ContextProjectionID = strings.TrimSpace(record.ContextProjectionID)
	record.RequestedModel = strings.TrimSpace(record.RequestedModel)
	record.Model = strings.TrimSpace(record.Model)
	record.Error = strings.TrimSpace(record.Error)
	return record
}

func llmEvidenceString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

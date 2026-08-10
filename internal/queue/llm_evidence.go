package queue

import (
	"context"
	"encoding/json"
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
		calls.user_prompt, calls.request_sha256, calls.response_format,
		calls.response_schema, calls.context_tokens, calls.max_output_tokens,
		calls.thinking_enabled, calls.response, calls.response_sha256, calls.status,
		calls.error, calls.latency_ms, calls.created_at`
)

type LLMCallEvidenceStatus string

const (
	LLMEvidencePreparationFailed LLMCallEvidenceStatus = "preparation_failed"
	LLMEvidenceGenerationFailed  LLMCallEvidenceStatus = "generation_failed"
	LLMEvidenceEmptyResponse     LLMCallEvidenceStatus = "empty_response"
	LLMEvidenceSucceeded         LLMCallEvidenceStatus = "succeeded"
)

type LLMCallEvidenceRecord struct {
	Authority           model.StepAttemptAuthority
	StepID              int64
	Scope               string
	WorkID              string
	WorkKind            string
	ContextProjectionID string
	RequestedModel      string
	Model               string
	Attempt             int
	SystemPrompt        string
	UserPrompt          string
	ResponseFormat      string
	ResponseSchema      map[string]any
	ContextTokens       int
	MaxOutputTokens     int
	ThinkingEnabled     bool
	Response            string
	Status              LLMCallEvidenceStatus
	Error               string
	LatencyMS           int64
}

type LLMCallEvidence struct {
	ID                  int64                 `json:"id"`
	JobID               int64                 `json:"job_id"`
	JobGeneration       int64                 `json:"job_generation"`
	StepID              int64                 `json:"step_id"`
	StepAttempt         int64                 `json:"step_attempt"`
	WorkerID            string                `json:"worker_id"`
	Scope               string                `json:"scope"`
	WorkID              string                `json:"work_id,omitempty"`
	WorkKind            string                `json:"work_kind,omitempty"`
	ContextProjectionID string                `json:"context_projection_id,omitempty"`
	RequestedModel      string                `json:"requested_model"`
	Model               string                `json:"model"`
	Attempt             int                   `json:"attempt"`
	SystemPrompt        string                `json:"system_prompt"`
	UserPrompt          string                `json:"user_prompt"`
	RequestSHA256       string                `json:"request_sha256"`
	ResponseFormat      string                `json:"response_format"`
	ResponseSchema      json.RawMessage       `json:"response_schema,omitempty"`
	ContextTokens       int                   `json:"context_tokens"`
	MaxOutputTokens     int                   `json:"max_output_tokens"`
	ThinkingEnabled     bool                  `json:"thinking_enabled"`
	Response            string                `json:"response,omitempty"`
	ResponseSHA256      string                `json:"response_sha256,omitempty"`
	Status              LLMCallEvidenceStatus `json:"status"`
	Error               string                `json:"error,omitempty"`
	LatencyMS           int64                 `json:"latency_ms"`
	CreatedAt           time.Time             `json:"created_at"`
}

func (r *Repository) RecordLLMCallEvidence(ctx context.Context, record LLMCallEvidenceRecord) (LLMCallEvidence, error) {
	record = normalizeLLMCallEvidenceRecord(record)
	schema, requestHash, err := validateAndHashLLMCallEvidenceRecord(record)
	if err != nil {
		return LLMCallEvidence{}, err
	}
	if r == nil || r.pool == nil {
		return LLMCallEvidence{}, fmt.Errorf("LLM call evidence requires PostgreSQL")
	}
	if record.StepID != record.Authority.StepID {
		return LLMCallEvidence{}, fmt.Errorf("%w: LLM evidence step disagrees with attempt", ErrStaleStepAttempt)
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return LLMCallEvidence{}, err
	}
	defer tx.Rollback(ctx)
	if jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, record.Authority); err != nil {
		return LLMCallEvidence{}, err
	} else if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
		return LLMCallEvidence{}, fmt.Errorf("%w: LLM evidence attempt is not running", ErrStaleStepAttempt)
	}
	response, responseHash := optionalLLMResponse(record.Response)
	var evidence LLMCallEvidence
	err = scanLLMCallEvidence(tx.QueryRow(ctx, `
		INSERT INTO llm_call_evidence (
			job_id, job_generation, step_id, scope, work_id, work_kind, context_projection_id,
			requested_model, model, attempt,
			system_prompt, user_prompt, request_sha256, response_format, response_schema,
			context_tokens, max_output_tokens, thinking_enabled, response, response_sha256, status, error, latency_ms,
			step_attempt, worker_id
		)
		SELECT steps.job_id, steps.generation, steps.id, $2, NULLIF($3,''), NULLIF($4,''),
		       NULLIF($5,''), $6, $7, $8, $9, $10, $11, $12, $13::jsonb, $14, $15,
		       $16, $17, $18, $19, NULLIF($20,''), $21, $24, $25
		FROM job_steps AS steps
		WHERE steps.id=$1 AND steps.job_id=$22 AND steps.generation=$23
		  AND steps.current_attempt=$24 AND steps.worker_id=$25 AND steps.status=$26
		RETURNING id, job_id, job_generation, step_id, step_attempt, worker_id,
		          scope, work_id, work_kind,
		          context_projection_id, requested_model, model,
		          attempt, system_prompt, user_prompt, request_sha256, response_format,
		          response_schema, context_tokens, max_output_tokens, thinking_enabled, response,
		          response_sha256, status, error, latency_ms, created_at
	`, record.StepID, record.Scope, record.WorkID, record.WorkKind, record.ContextProjectionID, record.RequestedModel,
		record.Model, record.Attempt, record.SystemPrompt, record.UserPrompt, requestHash,
		record.ResponseFormat, schema, record.ContextTokens, record.MaxOutputTokens,
		record.ThinkingEnabled, response, responseHash, string(record.Status), record.Error,
		record.LatencyMS, record.Authority.JobID, record.Authority.Generation,
		record.Authority.Attempt, record.Authority.WorkerID, model.StepStatusRunning), &evidence)
	if err != nil {
		return LLMCallEvidence{}, fmt.Errorf("record exact LLM call evidence: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return LLMCallEvidence{}, err
	}
	return evidence, nil
}

type llmEvidenceScanner interface {
	Scan(dest ...any) error
}

func scanLLMCallEvidence(scanner llmEvidenceScanner, evidence *LLMCallEvidence) error {
	var workID, workKind, projectionID, response, responseHash, errorText *string
	var schema []byte
	if err := scanner.Scan(
		&evidence.ID, &evidence.JobID, &evidence.JobGeneration, &evidence.StepID,
		&evidence.StepAttempt, &evidence.WorkerID,
		&evidence.Scope, &workID, &workKind, &projectionID,
		&evidence.RequestedModel, &evidence.Model, &evidence.Attempt, &evidence.SystemPrompt,
		&evidence.UserPrompt, &evidence.RequestSHA256, &evidence.ResponseFormat, &schema,
		&evidence.ContextTokens, &evidence.MaxOutputTokens, &evidence.ThinkingEnabled, &response, &responseHash,
		&evidence.Status, &errorText, &evidence.LatencyMS, &evidence.CreatedAt,
	); err != nil {
		return err
	}
	evidence.WorkID = llmEvidenceString(workID)
	evidence.WorkKind = llmEvidenceString(workKind)
	evidence.ContextProjectionID = llmEvidenceString(projectionID)
	evidence.ResponseSchema = append(json.RawMessage(nil), schema...)
	evidence.Response = llmEvidenceString(response)
	evidence.ResponseSHA256 = llmEvidenceString(responseHash)
	evidence.Error = llmEvidenceString(errorText)
	return nil
}

func normalizeLLMCallEvidenceRecord(record LLMCallEvidenceRecord) LLMCallEvidenceRecord {
	record.Scope = strings.TrimSpace(record.Scope)
	record.WorkID = strings.TrimSpace(record.WorkID)
	record.WorkKind = strings.TrimSpace(record.WorkKind)
	record.ContextProjectionID = strings.TrimSpace(record.ContextProjectionID)
	record.RequestedModel = strings.TrimSpace(record.RequestedModel)
	record.Model = strings.TrimSpace(record.Model)
	record.ResponseFormat = strings.ToLower(strings.TrimSpace(record.ResponseFormat))
	if record.ResponseFormat == "" {
		record.ResponseFormat = "text"
	}
	record.Error = strings.TrimSpace(record.Error)
	return record
}

func llmEvidenceString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

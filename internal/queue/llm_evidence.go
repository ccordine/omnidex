package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const maxLLMCallEvidencePageSize = 500

type LLMCallEvidenceStatus string

const (
	LLMEvidencePreparationFailed LLMCallEvidenceStatus = "preparation_failed"
	LLMEvidenceGenerationFailed  LLMCallEvidenceStatus = "generation_failed"
	LLMEvidenceEmptyResponse     LLMCallEvidenceStatus = "empty_response"
	LLMEvidenceSucceeded         LLMCallEvidenceStatus = "succeeded"
)

type LLMCallEvidenceRecord struct {
	StepID          int64
	Scope           string
	WorkID          string
	WorkKind        string
	RequestedModel  string
	Model           string
	Attempt         int
	SystemPrompt    string
	UserPrompt      string
	ResponseFormat  string
	ResponseSchema  map[string]any
	ContextTokens   int
	MaxOutputTokens int
	Response        string
	Status          LLMCallEvidenceStatus
	Error           string
	LatencyMS       int64
}

type LLMCallEvidence struct {
	ID              int64
	JobID           int64
	StepID          int64
	Scope           string
	WorkID          string
	WorkKind        string
	RequestedModel  string
	Model           string
	Attempt         int
	SystemPrompt    string
	UserPrompt      string
	RequestSHA256   string
	ResponseFormat  string
	ResponseSchema  json.RawMessage
	ContextTokens   int
	MaxOutputTokens int
	Response        string
	ResponseSHA256  string
	Status          LLMCallEvidenceStatus
	Error           string
	LatencyMS       int64
	CreatedAt       time.Time
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
	response, responseHash := optionalLLMResponse(record.Response)
	var evidence LLMCallEvidence
	err = scanLLMCallEvidence(r.pool.QueryRow(ctx, `
		INSERT INTO llm_call_evidence (
			job_id, step_id, scope, work_id, work_kind, requested_model, model, attempt,
			system_prompt, user_prompt, request_sha256, response_format, response_schema,
			context_tokens, max_output_tokens, response, response_sha256, status, error, latency_ms
		)
		SELECT steps.job_id, steps.id, $2, NULLIF($3,''), NULLIF($4,''), $5, $6, $7,
		       $8, $9, $10, $11, $12::jsonb, $13, $14, $15, $16, $17, NULLIF($18,''), $19
		FROM job_steps AS steps
		WHERE steps.id=$1
		RETURNING id, job_id, step_id, scope, work_id, work_kind, requested_model, model,
		          attempt, system_prompt, user_prompt, request_sha256, response_format,
		          response_schema, context_tokens, max_output_tokens, response,
		          response_sha256, status, error, latency_ms, created_at
	`, record.StepID, record.Scope, record.WorkID, record.WorkKind, record.RequestedModel,
		record.Model, record.Attempt, record.SystemPrompt, record.UserPrompt, requestHash,
		record.ResponseFormat, schema, record.ContextTokens, record.MaxOutputTokens,
		response, responseHash, string(record.Status), record.Error, record.LatencyMS), &evidence)
	if err != nil {
		return LLMCallEvidence{}, fmt.Errorf("record exact LLM call evidence: %w", err)
	}
	return evidence, nil
}

func (r *Repository) ListLLMCallEvidenceForJob(ctx context.Context, jobID int64, limit, offset int) ([]LLMCallEvidence, error) {
	if jobID < 1 {
		return nil, fmt.Errorf("LLM call evidence requires a positive job id")
	}
	if limit < 1 || limit > maxLLMCallEvidencePageSize {
		return nil, fmt.Errorf("LLM call evidence limit must be between 1 and %d", maxLLMCallEvidencePageSize)
	}
	if offset < 0 {
		return nil, fmt.Errorf("LLM call evidence offset cannot be negative")
	}
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("LLM call evidence requires PostgreSQL")
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, job_id, step_id, scope, work_id, work_kind, requested_model, model,
		       attempt, system_prompt, user_prompt, request_sha256, response_format,
		       response_schema, context_tokens, max_output_tokens, response,
		       response_sha256, status, error, latency_ms, created_at
		FROM llm_call_evidence
		WHERE job_id=$1
		ORDER BY id ASC
		LIMIT $2 OFFSET $3
	`, jobID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list exact LLM call evidence: %w", err)
	}
	defer rows.Close()
	evidence := make([]LLMCallEvidence, 0, limit)
	for rows.Next() {
		var item LLMCallEvidence
		if err := scanLLMCallEvidence(rows, &item); err != nil {
			return nil, fmt.Errorf("scan exact LLM call evidence: %w", err)
		}
		evidence = append(evidence, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate exact LLM call evidence: %w", err)
	}
	return evidence, nil
}

type llmEvidenceScanner interface {
	Scan(dest ...any) error
}

func scanLLMCallEvidence(scanner llmEvidenceScanner, evidence *LLMCallEvidence) error {
	var workID, workKind, response, responseHash, errorText *string
	var schema []byte
	if err := scanner.Scan(
		&evidence.ID, &evidence.JobID, &evidence.StepID, &evidence.Scope, &workID, &workKind,
		&evidence.RequestedModel, &evidence.Model, &evidence.Attempt, &evidence.SystemPrompt,
		&evidence.UserPrompt, &evidence.RequestSHA256, &evidence.ResponseFormat, &schema,
		&evidence.ContextTokens, &evidence.MaxOutputTokens, &response, &responseHash,
		&evidence.Status, &errorText, &evidence.LatencyMS, &evidence.CreatedAt,
	); err != nil {
		return err
	}
	evidence.WorkID = llmEvidenceString(workID)
	evidence.WorkKind = llmEvidenceString(workKind)
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
	record.RequestedModel = strings.TrimSpace(record.RequestedModel)
	record.Model = strings.TrimSpace(record.Model)
	record.ResponseFormat = strings.ToLower(strings.TrimSpace(record.ResponseFormat))
	if record.ResponseFormat == "" {
		record.ResponseFormat = "text"
	}
	record.Error = strings.TrimSpace(record.Error)
	return record
}

func validateAndHashLLMCallEvidenceRecord(record LLMCallEvidenceRecord) (any, string, error) {
	if err := validateLLMCallEvidenceRecord(record); err != nil {
		return nil, "", err
	}
	var schema any
	var schemaJSON json.RawMessage
	if record.ResponseSchema != nil {
		raw, err := json.Marshal(record.ResponseSchema)
		if err != nil {
			return nil, "", fmt.Errorf("encode LLM evidence response schema: %w", err)
		}
		schema = string(raw)
		schemaJSON = raw
	}
	digestInput := struct {
		RequestedModel  string          `json:"requested_model"`
		Model           string          `json:"model"`
		SystemPrompt    string          `json:"system_prompt"`
		UserPrompt      string          `json:"user_prompt"`
		ResponseFormat  string          `json:"response_format"`
		ResponseSchema  json.RawMessage `json:"response_schema,omitempty"`
		ContextTokens   int             `json:"context_tokens"`
		MaxOutputTokens int             `json:"max_output_tokens"`
	}{record.RequestedModel, record.Model, record.SystemPrompt, record.UserPrompt,
		record.ResponseFormat, schemaJSON, record.ContextTokens, record.MaxOutputTokens}
	raw, err := json.Marshal(digestInput)
	if err != nil {
		return nil, "", fmt.Errorf("hash exact LLM request: %w", err)
	}
	return schema, llmEvidenceSHA256(string(raw)), nil
}

func validateLLMCallEvidenceRecord(record LLMCallEvidenceRecord) error {
	if record.StepID < 1 || record.Attempt < 1 {
		return fmt.Errorf("LLM call evidence requires positive step and attempt identities")
	}
	if record.Scope == "" || record.RequestedModel == "" || record.Model == "" {
		return fmt.Errorf("LLM call evidence requires scope, requested model, and effective model")
	}
	if strings.TrimSpace(record.SystemPrompt) == "" || strings.TrimSpace(record.UserPrompt) == "" {
		return fmt.Errorf("LLM call evidence requires the exact system and user prompts")
	}
	if (record.WorkID == "") != (record.WorkKind == "") {
		return fmt.Errorf("LLM call evidence work id and kind must be present together")
	}
	if record.WorkID != "" && (len(record.WorkID) != 64 || !llmEvidenceLowerHex(record.WorkID)) {
		return fmt.Errorf("LLM call evidence work id must be one SHA-256 digest")
	}
	if record.ResponseFormat != "text" && record.ResponseFormat != "json" {
		return fmt.Errorf("LLM call evidence response format %q is unsupported", record.ResponseFormat)
	}
	if record.ResponseSchema != nil && record.ResponseFormat != "json" {
		return fmt.Errorf("LLM call evidence schema requires JSON response format")
	}
	if record.ContextTokens < 1 || record.MaxOutputTokens < 1 || record.LatencyMS < 0 {
		return fmt.Errorf("LLM call evidence requires positive inference limits and non-negative latency")
	}
	switch record.Status {
	case LLMEvidenceSucceeded:
		if strings.TrimSpace(record.Response) == "" || record.Error != "" {
			return fmt.Errorf("successful LLM call evidence requires a response and no error")
		}
	case LLMEvidencePreparationFailed:
		if record.Response != "" || record.Error == "" {
			return fmt.Errorf("preparation failure evidence requires an error and no response")
		}
	case LLMEvidenceGenerationFailed:
		if record.Error == "" {
			return fmt.Errorf("generation failure evidence requires an error")
		}
	case LLMEvidenceEmptyResponse:
		if strings.TrimSpace(record.Response) != "" || record.Error == "" {
			return fmt.Errorf("empty response evidence requires an error and no non-whitespace response")
		}
	default:
		return fmt.Errorf("LLM call evidence status %q is unsupported", record.Status)
	}
	return nil
}

func optionalLLMResponse(response string) (any, any) {
	if response == "" {
		return nil, nil
	}
	return response, llmEvidenceSHA256(response)
}

func llmEvidenceSHA256(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func llmEvidenceLowerHex(value string) bool {
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

func llmEvidenceString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

func validateAndHashLLMCallEvidenceRecord(record LLMCallEvidenceRecord) (any, string, error) {
	if err := validateLLMCallEvidenceRecord(record); err != nil {
		return nil, "", err
	}
	digestInput := struct {
		ContextProjectionID string `json:"context_projection_id,omitempty"`
		RequestedModel      string `json:"requested_model"`
		Model               string `json:"model"`
		SystemPrompt        string `json:"system_prompt"`
		UserPrompt          string `json:"user_prompt"`
		ResponseFormat      string `json:"response_format"`
		ContextTokens       int    `json:"context_tokens"`
		MaxOutputTokens     int    `json:"max_output_tokens"`
		ThinkingEnabled     bool   `json:"thinking_enabled"`
	}{
		ContextProjectionID: record.ContextProjectionID,
		RequestedModel:      record.RequestedModel, Model: record.Model,
		SystemPrompt: record.SystemPrompt, UserPrompt: record.UserPrompt,
		ResponseFormat: record.ResponseFormat,
		ContextTokens:  record.ContextTokens, MaxOutputTokens: record.MaxOutputTokens,
		ThinkingEnabled: record.ThinkingEnabled,
	}
	raw, err := json.Marshal(digestInput)
	if err != nil {
		return nil, "", fmt.Errorf("hash exact LLM request: %w", err)
	}
	return nil, llmEvidenceSHA256(string(raw)), nil
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
	if record.StationCallOpeningID < 1 {
		return fmt.Errorf("LLM call evidence requires one persisted station call opening")
	}
	if record.WorkID != "" && (len(record.WorkID) != 64 || !llmEvidenceLowerHex(record.WorkID)) {
		return fmt.Errorf("LLM call evidence work id must be one SHA-256 digest")
	}
	if record.ContextProjectionID != "" {
		if record.WorkID == "" || !validContextProjectionID(record.ContextProjectionID) {
			return fmt.Errorf("LLM call evidence projection requires exact work and projection identities")
		}
	}
	if record.ResponseFormat != "text" {
		return fmt.Errorf("LLM call evidence requires raw text response format")
	}
	if record.ResponseSchema != nil {
		return fmt.Errorf("LLM call evidence forbids a response schema")
	}
	if record.ContextTokens < 1 || record.MaxOutputTokens < 1 ||
		record.MaxOutputTokens > record.ContextTokens || record.LatencyMS < 0 {
		return fmt.Errorf("LLM call evidence requires bounded inference limits and non-negative latency")
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

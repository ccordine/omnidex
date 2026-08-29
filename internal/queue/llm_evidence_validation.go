package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

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
	if record.ContextTokens < 1 || record.MaxOutputTokens < 1 ||
		record.MaxOutputTokens > record.ContextTokens || record.LatencyMS < 0 {
		return fmt.Errorf("LLM call evidence requires bounded inference limits and non-negative latency")
	}
	switch record.Status {
	case LLMEvidenceSucceeded:
		if strings.TrimSpace(record.Response) == "" || record.Error != "" {
			return fmt.Errorf("successful LLM call evidence requires a response and no error")
		}
	case LLMEvidenceGenerationFailed:
		if record.Error == "" {
			return fmt.Errorf("generation failure evidence requires an error")
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

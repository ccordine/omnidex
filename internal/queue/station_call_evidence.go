package queue

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

func llmEvidenceForStationCall(
	authority model.StepAttemptAuthority,
	gap StationGapOpening,
	opening StationCallOpening,
	receipt StationCallReceipt,
	requestedModel string,
	attempt int,
	latencyMS int64,
) (LLMCallEvidenceRecord, error) {
	if opening.ID < 1 || gap.ID != opening.GapOpeningID || gap.GapID != opening.GapID ||
		receipt.OpeningID != opening.ID ||
		gap.JobID != authority.JobID || gap.Generation != authority.Generation ||
		gap.StepID != authority.StepID || gap.StepAttempt != authority.Attempt ||
		gap.WorkerID != authority.WorkerID ||
		receipt.JobID != authority.JobID || receipt.Generation != authority.Generation ||
		receipt.StepID != authority.StepID || receipt.StepAttempt != authority.Attempt ||
		receipt.WorkerID != authority.WorkerID || receipt.GapID != opening.GapID {
		return LLMCallEvidenceRecord{}, fmt.Errorf("LLM evidence requires one exact station call receipt")
	}
	var generation llm.PreparedGeneration
	if err := json.Unmarshal(receipt.GenerationJSON, &generation); err != nil {
		return LLMCallEvidenceRecord{}, fmt.Errorf("decode durable station call receipt: %w", err)
	}
	var schema map[string]any
	format := "text"
	if string(gap.ResponseSchema) != "null" {
		if err := json.Unmarshal(gap.ResponseSchema, &schema); err != nil {
			return LLMCallEvidenceRecord{}, fmt.Errorf("decode station call response schema: %w", err)
		}
		format = "json"
	}
	status := LLMEvidenceSucceeded
	if receipt.Status != "succeeded" {
		status = LLMEvidenceGenerationFailed
	}
	return LLMCallEvidenceRecord{
		Authority: authority, StepID: authority.StepID, Scope: gap.Scope,
		WorkID: opening.GapID, WorkKind: gap.WorkKind, StationCallOpeningID: opening.ID,
		RequestedModel: requestedModel, Model: opening.Model, Attempt: attempt,
		SystemPrompt: gap.Prompt, UserPrompt: llm.MinimalGeneratePrompt,
		ResponseFormat: format, ResponseSchema: schema,
		ContextTokens: opening.ContextTokens, MaxOutputTokens: opening.MaxOutputTokens,
		Response: generation.Content, Status: status, Error: receipt.Error, LatencyMS: latencyMS,
	}, nil
}

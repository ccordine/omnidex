package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

func validateExactStationStaticCall(
	prompt string,
	schema map[string]any,
	contract llmResponseContract,
	selection llm.ProviderIdentitySelection,
) error {
	if err := selection.Validate(); err != nil {
		return fmt.Errorf("validate exact station selection before provider discovery: %w", err)
	}
	if err := contract.Protocol.Validate(); err != nil {
		return err
	}
	switch contract.Protocol {
	case llm.ExactPreparedProtocolStructuredV1:
		if contract.Format != llm.ResponseFormatJSON || schema == nil {
			return fmt.Errorf("exact structured station requires one JSON schema")
		}
	case llm.ExactPreparedProtocolRawTextV1:
		if contract.Format != "" || schema != nil {
			return fmt.Errorf("exact raw-text station forbids a response schema")
		}
	}
	input, err := llm.ExactPreparedModelInput(prompt, contract.PromptHint)
	if err != nil {
		return err
	}
	return llm.ValidateExactPreparedInputBudget(
		selection.NativeContextLimit,
		selection.NativeContextLimit-contract.MaxTokens,
		contract.MaxTokens,
		input,
		llm.MaxRawInputSpecialTokenReserve,
	)
}

func prepareExactStationCall(
	gap queue.StationGapOpening,
	contract llmResponseContract,
	modelName string,
	expected llm.ProviderIdentityExpectation,
) (llm.PreparedModel, error) {
	challengeScope, err := stationCallChallengeScope(gap, contract, modelName)
	if err != nil {
		return llm.PreparedModel{}, err
	}
	challenge, err := llm.DeriveProviderIdentityObservationChallenge(challengeScope, expected)
	if err != nil {
		return llm.PreparedModel{}, err
	}
	var schema map[string]any
	if string(gap.ResponseSchema) != "null" {
		if err := json.Unmarshal(gap.ResponseSchema, &schema); err != nil {
			return llm.PreparedModel{}, fmt.Errorf("decode durable station response schema: %w", err)
		}
	}
	temperature := float64(0)
	return llm.PreparedModel{
		Protocol: contract.Protocol, BaseModel: modelName, ContextModel: modelName,
		Prompt: gap.Prompt, PromptHint: llm.MinimalGeneratePrompt,
		MaxOutputTokens: gap.MaxOutputTokens, ContextTokens: gap.ContextTokens,
		ResponseFormat: contract.Format, ResponseSchema: schema,
		ThinkingEnabled: false, Temperature: &temperature,
		ProviderIdentityExpectation: &expected, ProviderObservationChallenge: challenge,
	}, nil
}

func stationCallChallengeScope(
	gap queue.StationGapOpening,
	contract llmResponseContract,
	modelName string,
) (string, error) {
	raw, err := exactjson.Canonical(struct {
		JobID                                                        int64
		Generation, StepID, StepAttempt                              int64
		WorkerID, GapID, Station, WorkID, WorkKind, ProjectionSHA256 string
		RendererVersion, Model, Protocol                             string
		ContextTokens, MaxOutputTokens                               int
	}{
		JobID: gap.JobID, Generation: gap.Generation, StepID: gap.StepID,
		StepAttempt: gap.StepAttempt, WorkerID: gap.WorkerID,
		GapID: gap.GapID, Station: string(gap.Station), WorkID: gap.WorkID,
		WorkKind: gap.WorkKind, ProjectionSHA256: gap.ProjectionSHA256,
		RendererVersion: gap.RendererVersion, Model: modelName,
		Protocol: string(contract.Protocol), ContextTokens: gap.ContextTokens,
		MaxOutputTokens: gap.MaxOutputTokens,
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return "station-call:" + hex.EncodeToString(digest[:]), nil
}

func ownStationDiscovery(observed llm.ObservedProviderIdentity) (llm.ObservedProviderIdentity, error) {
	evidence, err := llm.OwnBoundedProviderIdentityEvidence(observed.Evidence)
	if err != nil {
		return llm.ObservedProviderIdentity{}, fmt.Errorf("own bounded provider discovery: %w", err)
	}
	observed.Evidence = evidence
	return observed, nil
}

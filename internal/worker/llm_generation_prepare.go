package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

func validateExactStationStaticCall(
	prompt string,
	contract llmResponseContract,
	selection llm.ProviderIdentitySelection,
) error {
	if err := selection.Validate(); err != nil {
		return fmt.Errorf("validate exact station selection before provider discovery: %w", err)
	}
	if err := contract.Protocol.Validate(); err != nil {
		return err
	}
	if err := contract.OutputLimitMode.Validate(); err != nil {
		return err
	}
	if contract.OutputLimitMode != llm.ExactPreparedOutputLimitNatural {
		return fmt.Errorf("portable exact station requires natural output completion")
	}
	if contract.Protocol != llm.ExactPreparedProtocolRawTextV2 {
		return fmt.Errorf("exact portable station requires raw text")
	}
	input, err := llm.ExactPreparedModelInput(prompt, contract.PromptHint)
	if err != nil {
		return err
	}
	return llm.ValidateExactPreparedNaturalInputAuthority(
		selection.NativeContextLimit,
		input,
	)
}

func prepareExactStationCall(
	gap queue.StationGapOpening,
	contract llmResponseContract,
	modelName string,
	expected llm.ProviderIdentityExpectation,
	temperature *llm.ExactPreparedTemperature,
) (llm.PreparedModel, error) {
	if gap.OutputLimitMode != contract.OutputLimitMode {
		return llm.PreparedModel{}, fmt.Errorf(
			"durable station gap output-limit mode %q differs from response contract %q",
			gap.OutputLimitMode, contract.OutputLimitMode,
		)
	}
	if string(gap.ResponseSchema) != "null" {
		return llm.PreparedModel{}, fmt.Errorf("durable station gap contains a forbidden structured response schema")
	}
	transport, err := llm.ResolveExactPreparedTransport(expected)
	if err != nil {
		return llm.PreparedModel{}, err
	}
	if temperature == nil {
		temperature = transport.Temperature
	}
	stop, err := queue.ExpectedStationCallStopSequence(gap, expected)
	if err != nil {
		return llm.PreparedModel{}, err
	}
	responseFramingIdentity, err := llm.ExactPreparedResponseFramingIdentity(expected)
	if err != nil {
		return llm.PreparedModel{}, err
	}
	challengeScope, err := stationCallChallengeScope(
		gap, contract, modelName, responseFramingIdentity, stop, temperature,
	)
	if err != nil {
		return llm.PreparedModel{}, err
	}
	challenge, err := llm.DeriveProviderIdentityObservationChallenge(challengeScope, expected)
	if err != nil {
		return llm.PreparedModel{}, err
	}
	return llm.PreparedModel{
		Protocol: contract.Protocol, BaseModel: modelName, ContextModel: modelName,
		Prompt: gap.Prompt, PromptHint: llm.MinimalGeneratePrompt,
		MaxOutputTokens: gap.MaxOutputTokens, OutputLimitMode: gap.OutputLimitMode,
		ContextTokens:       gap.ContextTokens,
		RawTextStopSequence: stop,
		ThinkingEnabled:     transport.SeparateThinking, Temperature: temperature,
		ProviderIdentityExpectation: &expected, ProviderObservationChallenge: challenge,
	}, nil
}

func stationCallChallengeScope(
	gap queue.StationGapOpening,
	contract llmResponseContract,
	modelName string,
	responseFramingIdentity string,
	rawTextStopSequence string,
	temperature *llm.ExactPreparedTemperature,
) (string, error) {
	raw, err := exactjson.Canonical(struct {
		JobID                                                        int64
		Generation, StepID, StepAttempt                              int64
		WorkerID, GapID, Station, WorkID, WorkKind, ProjectionSHA256 string
		RendererVersion, Model, Protocol, ResponseFramingIdentity    string
		RawTextStopSequence                                          string
		OutputLimitMode                                              llm.ExactPreparedOutputLimitMode
		Temperature                                                  *llm.ExactPreparedTemperature
		ContextTokens, MaxOutputTokens                               int
	}{
		JobID: gap.JobID, Generation: gap.Generation, StepID: gap.StepID,
		StepAttempt: gap.StepAttempt, WorkerID: gap.WorkerID,
		GapID: gap.GapID, Station: string(gap.Station), WorkID: gap.WorkID,
		WorkKind: gap.WorkKind, ProjectionSHA256: gap.ProjectionSHA256,
		RendererVersion: gap.RendererVersion, Model: modelName,
		Protocol: string(contract.Protocol), ContextTokens: gap.ContextTokens,
		ResponseFramingIdentity: responseFramingIdentity,
		MaxOutputTokens:         gap.MaxOutputTokens, RawTextStopSequence: rawTextStopSequence,
		OutputLimitMode: gap.OutputLimitMode,
		Temperature:     temperature,
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

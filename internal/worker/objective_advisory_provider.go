package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/objectiveadvisory"
)

type exactObjectiveAdvisoryProvider struct {
	client        llm.ExactStationClient
	contextTokens int
}

func (provider exactObjectiveAdvisoryProvider) Generate(
	ctx context.Context,
	request objectiveadvisory.GenerateRequest,
) (objectiveadvisory.Generation, error) {
	if ctx == nil || provider.client == nil {
		return objectiveadvisory.Generation{}, fmt.Errorf(
			"objective advisory exact provider requires context and client",
		)
	}
	if err := ctx.Err(); err != nil {
		return objectiveadvisory.Generation{}, err
	}
	prompt, err := objectiveadvisory.BuildPrompt(request)
	if err != nil {
		return objectiveadvisory.Generation{}, err
	}
	if err := validateObjectiveAdvisorySampling(request.Source.Sampling); err != nil {
		return objectiveadvisory.Generation{}, err
	}
	if request.Source.Provider != llm.ExactPreparedProviderBackend {
		return objectiveadvisory.Generation{}, fmt.Errorf(
			"objective advisory requires exact provider %q, received %q",
			llm.ExactPreparedProviderBackend, request.Source.Provider,
		)
	}
	selection := llm.ProviderIdentitySelection{
		Model: request.Source.Model, NativeContextLimit: provider.contextTokens,
	}
	if err := validateObjectiveAdvisoryInputBudget(prompt, request.Source, selection); err != nil {
		return objectiveadvisory.Generation{}, err
	}
	discoveryScope, err := objectiveAdvisoryProviderScope("discovery", request)
	if err != nil {
		return objectiveadvisory.Generation{}, err
	}
	observed, err := llm.RequireDiscoveredProviderIdentityEvidence(
		ctx, provider.client, selection, discoveryScope,
	)
	if err != nil {
		return objectiveadvisory.Generation{}, fmt.Errorf(
			"discover objective advisory provider identity: %w", err,
		)
	}
	expected, err := llm.DeriveExactProviderIdentityExpectation(observed.Evidence, selection)
	if err != nil {
		return objectiveadvisory.Generation{}, fmt.Errorf(
			"derive objective advisory provider identity: %w", err,
		)
	}
	if err := llm.ValidateExactPreparedProvider(provider.client, expected); err != nil {
		return objectiveadvisory.Generation{}, err
	}
	prepared, err := prepareObjectiveAdvisoryCall(prompt, request, selection, expected)
	if err != nil {
		return objectiveadvisory.Generation{}, err
	}
	if err := provider.client.ValidateExactPreparedContract(prepared); err != nil {
		return objectiveadvisory.Generation{}, fmt.Errorf(
			"validate objective advisory exact request: %w", err,
		)
	}
	generated, callErr := provider.client.GeneratePreparedExact(ctx, prepared)
	owned, ownershipErr := llm.OwnBoundedPreparedGeneration(generated)
	if err := ctx.Err(); err != nil {
		return objectiveadvisory.Generation{}, err
	}
	if callErr != nil {
		return objectiveadvisory.Generation{}, errors.Join(
			fmt.Errorf("objective advisory provider call: %w", callErr),
			objectiveAdvisoryOwnershipError(ownershipErr),
		)
	}
	if ownershipErr != nil {
		return objectiveadvisory.Generation{}, objectiveAdvisoryOwnershipError(ownershipErr)
	}
	if err := validateObjectiveAdvisoryGeneration(owned, prepared, observed, expected); err != nil {
		return objectiveadvisory.Generation{}, err
	}
	return objectiveadvisory.Generation{
		FinalText: owned.Content, EffectiveProvider: expected.Backend,
		EffectiveModel: owned.ProviderResponseModel, ModelDigest: expected.Digest,
		Quantization: expected.Quantization, PromptTokens: owned.Usage.PromptEvalCount,
		OutputTokens: owned.Usage.EvalCount,
		Duration:     time.Duration(owned.Usage.TotalDurationNanos),
		FinishReason: owned.ProviderDoneReason,
	}, nil
}

func objectiveAdvisoryOwnershipError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("own bounded objective advisory result: %w", err)
}

func validateObjectiveAdvisorySampling(config objectiveadvisory.SamplingConfig) error {
	if config.Temperature != 0 || math.Signbit(config.Temperature) ||
		config.TopP != nil || config.Seed != nil {
		return fmt.Errorf("objective advisory production sampling requires exact temperature 0 with no top_p or seed")
	}
	return nil
}

func validateObjectiveAdvisoryInputBudget(
	prompt string,
	source objectiveadvisory.SourceConfig,
	selection llm.ProviderIdentitySelection,
) error {
	if err := selection.Validate(); err != nil {
		return err
	}
	input, err := llm.ExactPreparedModelInput(prompt, llm.MinimalGeneratePrompt)
	if err != nil {
		return err
	}
	return llm.ValidateExactPreparedNaturalInputAuthority(selection.NativeContextLimit, input)
}

func prepareObjectiveAdvisoryCall(
	prompt string,
	request objectiveadvisory.GenerateRequest,
	selection llm.ProviderIdentitySelection,
	expected llm.ProviderIdentityExpectation,
) (llm.PreparedModel, error) {
	scope, err := objectiveAdvisoryProviderScope("generation", request)
	if err != nil {
		return llm.PreparedModel{}, err
	}
	challenge, err := llm.DeriveProviderIdentityObservationChallenge(scope, expected)
	if err != nil {
		return llm.PreparedModel{}, err
	}
	transport, err := llm.ResolveExactPreparedTransport(expected)
	if err != nil {
		return llm.PreparedModel{}, err
	}
	temperature := float64(0)
	prepared := llm.PreparedModel{
		Protocol:  llm.ExactPreparedProtocolRawTextV1,
		BaseModel: selection.Model, ContextModel: selection.Model,
		PromptHint: llm.MinimalGeneratePrompt, Prompt: prompt,
		MaxOutputTokens: request.Source.Budget.MaxOutputTokens,
		OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
		ContextTokens:   selection.NativeContextLimit,
		ThinkingEnabled: transport.SeparateThinking, Temperature: &temperature,
		RawTextStopSequence:          llm.ExactPreparedObjectiveAdvisoryStopV1,
		ProviderIdentityExpectation:  &expected,
		ProviderObservationChallenge: challenge,
	}
	if _, err := llm.ExactPreparedRequestBytes(prepared); err != nil {
		return llm.PreparedModel{}, err
	}
	return prepared, nil
}

func validateObjectiveAdvisoryGeneration(
	generated llm.PreparedGeneration,
	prepared llm.PreparedModel,
	discovered llm.ObservedProviderIdentity,
	expected llm.ProviderIdentityExpectation,
) error {
	if err := generated.Validate(); err != nil {
		return fmt.Errorf("validate objective advisory exact response: %w", err)
	}
	wantRequest, err := llm.ExactPreparedRequestSHA256(prepared)
	if err != nil {
		return err
	}
	if generated.ProviderRequestSHA256 != wantRequest ||
		generated.ProviderResponseModel != expected.Model {
		return fmt.Errorf("objective advisory response differs from its exact request or model")
	}
	currentExpected, err := llm.DeriveExactProviderIdentityExpectation(
		generated.ProviderIdentityEvidence,
		llm.ProviderIdentitySelection{
			Model: expected.Model, NativeContextLimit: expected.NativeContextLimit,
		},
	)
	if err != nil || currentExpected != expected {
		return fmt.Errorf("objective advisory response changed its discovered provider identity")
	}
	current := llm.ObservedProviderIdentity{
		Attestation: discovered.Attestation, Observation: generated.ProviderObservation,
		Evidence: generated.ProviderIdentityEvidence,
	}
	if err := current.ValidateFor(llm.ProviderIdentityObservationRequest{
		Expectation: expected, ChallengeSHA256: prepared.ProviderObservationChallenge,
	}); err != nil {
		return fmt.Errorf("validate objective advisory response identity: %w", err)
	}
	return nil
}

func objectiveAdvisoryProviderScope(
	operation string,
	request objectiveadvisory.GenerateRequest,
) (string, error) {
	raw, err := exactjson.Canonical(struct {
		Operation, TriggerID, TriggerVersion, ProjectionID, ProjectionSHA256 string
		Source                                                               objectiveadvisory.SourceConfig
		RawTextStopSequence                                                  string
	}{
		Operation: operation, TriggerID: request.TriggerID,
		TriggerVersion: request.TriggerVersion, ProjectionID: request.Projection.ID,
		ProjectionSHA256: request.Projection.RenderedSHA256, Source: request.Source,
		RawTextStopSequence: llm.ExactPreparedObjectiveAdvisoryStopV1,
	})
	if err != nil {
		return "", fmt.Errorf("encode objective advisory provider scope: %w", err)
	}
	digest := sha256.Sum256(raw)
	return "objective-advisory-" + operation + ":" + hex.EncodeToString(digest[:]), nil
}

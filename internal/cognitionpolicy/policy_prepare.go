package cognitionpolicy

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/llm"
)

func (policy *Policy) prepareModelCall(
	ctx context.Context,
	attempt CallAttempt,
	snapshot cognition.RuntimeSnapshot,
) (llm.PreparedModel, error) {
	if attempt.RuntimeBudget.MaxOutputTokens > policy.brain.Ref.Sampling.MaxOutputTokens {
		return llm.PreparedModel{}, fmt.Errorf("runtime output limit exceeds the frozen cognition station ceiling")
	}
	prepared, err := policy.client.PrepareContextModel(ctx, policy.brain.Ref.Model, attempt.Envelope)
	if err != nil {
		return llm.PreparedModel{}, fmt.Errorf("prepare exact cognition model context: %w", err)
	}
	if prepared.BaseModel != policy.brain.Ref.Model || prepared.ContextModel != policy.brain.Ref.Model ||
		prepared.Prompt != attempt.Envelope {
		policy.client.CleanupPreparedModel(prepared)
		return llm.PreparedModel{}, fmt.Errorf("prepared model changed the frozen model or exact envelope")
	}
	schemaRaw, err := decisionSchemaJSON(snapshot.ActionCatalog())
	if err != nil {
		policy.client.CleanupPreparedModel(prepared)
		return llm.PreparedModel{}, err
	}
	var schema map[string]any
	if err := json.Unmarshal(schemaRaw, &schema); err != nil {
		policy.client.CleanupPreparedModel(prepared)
		return llm.PreparedModel{}, fmt.Errorf("decode exact cognition response schema: %w", err)
	}
	zero := 0.0
	prepared.PromptHint = llm.MinimalGeneratePrompt
	prepared.MaxOutputTokens = attempt.RuntimeBudget.MaxOutputTokens
	prepared.ContextTokens = policy.brain.Ref.NativeContextLimit
	prepared.ResponseFormat = llm.ResponseFormatJSON
	prepared.ResponseSchema = schema
	prepared.ThinkingEnabled = false
	prepared.Temperature = &zero
	expected, err := attempt.Brain.ProviderExpectation()
	if err != nil {
		policy.client.CleanupPreparedModel(prepared)
		return llm.PreparedModel{}, err
	}
	challenge, err := callProviderObservationChallenge(attempt, expected)
	if err != nil {
		policy.client.CleanupPreparedModel(prepared)
		return llm.PreparedModel{}, err
	}
	prepared.ProviderIdentityExpectation = &expected
	prepared.ProviderObservationChallenge = challenge
	if err := llm.ValidateResponseContract(prepared); err != nil {
		policy.client.CleanupPreparedModel(prepared)
		return llm.PreparedModel{}, err
	}
	rawInput, err := llm.ExactPreparedModelInput(prepared.Prompt, prepared.PromptHint)
	if err != nil {
		policy.client.CleanupPreparedModel(prepared)
		return llm.PreparedModel{}, err
	}
	if err := llm.ValidateExactPreparedInputBudget(
		prepared.ContextTokens,
		attempt.RuntimeBudget.MaxInputTokens,
		prepared.MaxOutputTokens,
		rawInput,
		policy.brain.Ref.Sampling.InputSpecialTokenReserve,
	); err != nil {
		policy.client.CleanupPreparedModel(prepared)
		return llm.PreparedModel{}, err
	}
	if err := policy.exactClient.ValidateExactPreparedContract(prepared); err != nil {
		policy.client.CleanupPreparedModel(prepared)
		return llm.PreparedModel{}, fmt.Errorf("provider rejected exact prepared contract: %w", err)
	}
	contractSHA, err := preparedResponseContractSHA(prepared)
	if err != nil || contractSHA != attempt.ResponseContractSHA256 {
		policy.client.CleanupPreparedModel(prepared)
		return llm.PreparedModel{}, fmt.Errorf("prepared response contract differs from the reserved call")
	}
	return prepared, nil
}

func validatePreparedGenerationProvider(
	attempt CallAttempt,
	generation llm.PreparedGeneration,
) error {
	if err := validateObservedProviderForAttempt(
		attempt, generation.ProviderObservation, generation.ProviderIdentityEvidence,
	); err != nil {
		return err
	}
	if generation.ProviderResponseDisposition == llm.ProviderResponseSucceeded &&
		generation.ProviderResponseModel != attempt.Brain.Model {
		return fmt.Errorf("provider response model differs from the exact cognition brain")
	}
	return nil
}

func validateObservedProviderForAttempt(
	attempt CallAttempt,
	observation llm.ProviderIdentityObservation,
	evidence llm.ProviderIdentityEvidence,
) error {
	expected, err := attempt.Brain.ProviderExpectation()
	if err != nil {
		return err
	}
	challenge, err := callProviderObservationChallenge(attempt, expected)
	if err != nil {
		return err
	}
	observed := llm.ObservedProviderIdentity{
		Attestation: attempt.ProviderAttestation, Observation: observation, Evidence: evidence,
	}
	return observed.ValidateFor(llm.ProviderIdentityObservationRequest{
		Expectation: expected, ChallengeSHA256: challenge,
	})
}

func preparedModelSHA(prepared llm.PreparedModel) (string, error) {
	raw, err := json.Marshal(prepared)
	if err != nil {
		return "", err
	}
	return policySHA256(string(raw)), nil
}

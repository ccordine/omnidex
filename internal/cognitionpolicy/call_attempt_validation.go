package cognitionpolicy

import (
	"fmt"

	"github.com/gryph/omnidex/internal/llm"
)

func (attempt CallAttempt) Validate() error {
	if attempt.Schema != CallAttemptSchemaV3 || attempt.ID != callAttemptID(attempt) {
		return fmt.Errorf("%w: call attempt identity is invalid", ErrInvalidEvidence)
	}
	if err := attempt.Actor.Validate(); err != nil {
		return fmt.Errorf("%w: actor: %v", ErrInvalidEvidence, err)
	}
	if !validPolicySHA256(attempt.SnapshotSHA256) || attempt.ExpectedRevision.EpisodeID == "" ||
		attempt.ObligationID == "" || attempt.ExpectedRevision.Validate() != nil ||
		attempt.RuntimeBudget.Validate() != nil || attempt.RuntimeBudget.RemainingPolicyCalls == 0 ||
		attempt.ContextProjection.Validate() != nil || attempt.Brain.Validate() != nil {
		return fmt.Errorf("%w: snapshot, budget, projection, or brain authority is invalid", ErrInvalidEvidence)
	}
	expected, err := attempt.Brain.ProviderExpectation()
	if err != nil || attempt.ProviderAttestation.ValidateFor(expected) != nil ||
		attempt.HostHardwareAttestation.Validate() != nil {
		return fmt.Errorf("%w: frozen provider or host authority is invalid", ErrInvalidEvidence)
	}
	if err := validateCallInput(attempt); err != nil {
		return err
	}
	return nil
}

func validateCallInput(attempt CallAttempt) error {
	modelInput, err := llm.ExactPreparedModelInput(attempt.Envelope, attempt.PromptHint)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidEvidence, err)
	}
	visibleBytes := len(modelInput)
	tokenUpperBound, err := llm.ModelInputTokenUpperBound(
		modelInput, attempt.Brain.Sampling.InputSpecialTokenReserve,
	)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidEvidence, err)
	}
	if attempt.EnvelopeRendererVersion != RendererVersionV2 ||
		attempt.EnvelopeTokenEstimator != policyTokenEstimator ||
		attempt.EnvelopeEstimatedTokens != estimatePolicyTokens(attempt.EnvelopeBytes) ||
		attempt.EnvelopeBytes != len(attempt.Envelope) ||
		attempt.PromptHint != llm.MinimalGeneratePrompt ||
		attempt.PromptHintBytes != len(attempt.PromptHint) ||
		attempt.PromptHintSHA256 != policySHA256(attempt.PromptHint) ||
		attempt.ModelVisibleInputBytes != visibleBytes ||
		attempt.ModelVisibleEstimatedTokens != estimatePolicyTokens(visibleBytes) ||
		attempt.ModelInputTokenUpperBound != tokenUpperBound ||
		attempt.ModelVisibleInputSHA256 != modelVisibleInputSHA(attempt) ||
		attempt.ModelVisibleInputBytes > attempt.RuntimeBudget.MaxInputBytes ||
		attempt.ModelInputTokenUpperBound > attempt.RuntimeBudget.MaxInputTokens ||
		attempt.ModelVisibleInputBytes > attempt.Brain.ContextCeilingBytes ||
		attempt.RuntimeBudget.MaxInputTokens+attempt.RuntimeBudget.MaxOutputTokens >
			attempt.Brain.NativeContextLimit ||
		attempt.RuntimeBudget.MaxOutputTokens > attempt.Brain.Sampling.MaxOutputTokens ||
		!validBoundedText(attempt.Envelope, MaxEnvelopeBytes) ||
		policySHA256(attempt.Envelope) != attempt.EnvelopeSHA256 ||
		!validPolicySHA256(attempt.ResponseContractSHA256) ||
		!validPolicySHA256(attempt.ExpectedProviderRequestSHA256) {
		return fmt.Errorf("%w: model-visible input authority is invalid", ErrInvalidEvidence)
	}
	return nil
}

func modelVisibleInputSHA(attempt CallAttempt) string {
	raw, err := llm.ExactPreparedModelInput(attempt.Envelope, attempt.PromptHint)
	if err != nil {
		panic(fmt.Sprintf("render model-visible input identity: %v", err))
	}
	return policySHA256(raw)
}

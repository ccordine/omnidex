package llm

import (
	"context"
	"fmt"
)

// ProviderIdentitySelection contains only the experiment choices needed to
// discover a live generation provider. Provider-maintained identity fields are
// observed by the provider implementation and never copied from configuration.
type ProviderIdentitySelection struct {
	Model              string `json:"model"`
	NativeContextLimit int    `json:"native_context_limit"`
}

type ProviderIdentityEvidenceDiscoverer interface {
	DiscoverProviderIdentityEvidence(
		context.Context,
		ProviderIdentitySelection,
		string,
	) (ObservedProviderIdentity, error)
}

func (selection ProviderIdentitySelection) Validate() error {
	if !providerIdentityText(selection.Model, 256) ||
		selection.NativeContextLimit < MinInferenceContextTokens ||
		selection.NativeContextLimit > MaxInferenceContextTokens {
		return fmt.Errorf("provider identity discovery selection is invalid")
	}
	return nil
}

func RequireDiscoveredProviderIdentityEvidence(
	ctx context.Context,
	client Client,
	selection ProviderIdentitySelection,
	scope string,
) (ObservedProviderIdentity, error) {
	if ctx == nil || client == nil {
		return ObservedProviderIdentity{}, fmt.Errorf(
			"provider identity discovery requires context and client",
		)
	}
	if err := selection.Validate(); err != nil {
		return ObservedProviderIdentity{}, err
	}
	challenge, err := DeriveProviderIdentityDiscoveryChallenge(scope, selection)
	if err != nil {
		return ObservedProviderIdentity{}, err
	}
	discoverer, ok := client.(ProviderIdentityEvidenceDiscoverer)
	if !ok {
		return ObservedProviderIdentity{}, fmt.Errorf(
			"configured generation provider cannot discover raw live identity evidence",
		)
	}
	observed, err := discoverer.DiscoverProviderIdentityEvidence(ctx, selection, challenge)
	if err != nil {
		if evidenceErr := observed.Evidence.ValidateRequests(selection); evidenceErr != nil {
			return observed, fmt.Errorf("provider identity discovery failure lacks exact raw evidence: %v: %w", evidenceErr, err)
		}
		return observed, err
	}
	expected, err := DeriveExactProviderIdentityExpectation(observed.Evidence, selection)
	if err != nil {
		return observed, err
	}
	if err := observed.ValidateFor(ProviderIdentityObservationRequest{
		Expectation: expected, ChallengeSHA256: challenge,
	}); err != nil {
		return observed, err
	}
	return observed, nil
}

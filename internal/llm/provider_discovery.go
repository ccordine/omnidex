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
	discoverer ProviderIdentityEvidenceDiscoverer,
	selection ProviderIdentitySelection,
	scope string,
) (ObservedProviderIdentity, error) {
	if ctx == nil || discoverer == nil {
		return ObservedProviderIdentity{}, fmt.Errorf(
			"provider identity discovery requires context and discoverer",
		)
	}
	if err := selection.Validate(); err != nil {
		return ObservedProviderIdentity{}, err
	}
	challenge, err := DeriveProviderIdentityDiscoveryChallenge(scope, selection)
	if err != nil {
		return ObservedProviderIdentity{}, err
	}
	observed, err := discoverer.DiscoverProviderIdentityEvidence(ctx, selection, challenge)
	owned, ownershipErr := OwnBoundedProviderIdentityEvidence(observed.Evidence)
	if ownershipErr != nil {
		return ObservedProviderIdentity{}, fmt.Errorf(
			"provider identity discovery exceeds its ownership bound: %w", ownershipErr,
		)
	}
	observed.Evidence = owned
	if err != nil {
		if evidenceErr := observed.Evidence.ValidateFailure(selection, nil); evidenceErr != nil {
			return observed, fmt.Errorf("provider identity discovery failure is not proven by exact raw evidence: %v: %w", evidenceErr, err)
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

package llm

import (
	"context"
	"fmt"
)

// ProviderIdentitySelection contains only the experiment choices needed to
// discover a live generation provider. Provider-maintained identity fields are
// observed by the provider implementation and never copied from configuration.
type ProviderIdentitySelection struct {
	Model              string                        `json:"model"`
	NativeContextLimit int                           `json:"native_context_limit"`
	ProfilePolicy      ProviderIdentityProfilePolicy `json:"profile_policy,omitempty"`
}

// ProviderIdentityProfilePolicy is code-owned provider admission authority.
// The empty value preserves the exact registered-profile policy used by every
// ordinary semantic and coding station. Roleplay policies narrow model use
// and context negotiation, but never bypass structural profile attestation or
// select a provider transport.
type ProviderIdentityProfilePolicy string

const (
	ProviderIdentityProfileRoleplayRawCompletion      ProviderIdentityProfilePolicy = "roleplay_raw_completion"
	ProviderIdentityProfileRoleplaySemanticCompletion ProviderIdentityProfilePolicy = "roleplay_semantic_completion"
)

func (policy ProviderIdentityProfilePolicy) Validate() error {
	switch policy {
	case "", ProviderIdentityProfileRoleplayRawCompletion,
		ProviderIdentityProfileRoleplaySemanticCompletion:
		return nil
	default:
		return fmt.Errorf("provider identity profile policy is not registered")
	}
}

type ProviderIdentityEvidenceDiscoverer interface {
	DiscoverProviderIdentityEvidence(
		context.Context,
		ProviderIdentitySelection,
		string,
	) (ObservedProviderIdentity, error)
}

// RoleplayCompletionContextResolver deterministically reads one local model's
// native context metadata. It is separate from ExactStationClient so providers
// cannot silently acquire this roleplay-only capability.
type RoleplayCompletionContextResolver interface {
	ResolveRoleplayCompletionContext(context.Context, string, int) (int, error)
}

func (selection ProviderIdentitySelection) Validate() error {
	if !providerIdentityText(selection.Model, 256) {
		return fmt.Errorf("provider identity discovery selection is invalid")
	}
	if err := selection.ProfilePolicy.Validate(); err != nil {
		return err
	}
	if selection.ProfilePolicy != "" {
		if err := ValidateRoleplayCompletionContextTokens(selection.NativeContextLimit); err != nil {
			return fmt.Errorf("provider identity discovery selection is invalid: %w", err)
		}
		return nil
	}
	if err := ValidateInferenceContextTokens(selection.NativeContextLimit); err != nil {
		return fmt.Errorf("provider identity discovery selection is invalid: %w", err)
	}
	return nil
}

func ProviderIdentitySelectionForExpectation(
	expected ProviderIdentityExpectation,
) (ProviderIdentitySelection, error) {
	if err := ValidateExactPreparedProviderExpectation(expected); err != nil {
		return ProviderIdentitySelection{}, err
	}
	return ProviderIdentitySelectionForProfile(
		expected.Model, expected.NativeContextLimit, expected.TokenizerProfile,
	)
}

func ProviderIdentitySelectionForProfile(
	model string,
	nativeContextLimit int,
	tokenizerProfile string,
) (ProviderIdentitySelection, error) {
	if _, err := exactProviderModelProfileByID(tokenizerProfile); err != nil {
		return ProviderIdentitySelection{}, err
	}
	selection := ProviderIdentitySelection{
		Model: model, NativeContextLimit: nativeContextLimit,
	}
	return selection, selection.Validate()
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

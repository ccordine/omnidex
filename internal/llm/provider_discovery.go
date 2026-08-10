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

type ProviderIdentityDiscoverer interface {
	DiscoverProviderIdentity(
		context.Context,
		ProviderIdentitySelection,
	) (ProviderIdentityAttestation, error)
}

func (selection ProviderIdentitySelection) Validate() error {
	if !providerIdentityText(selection.Model, 256) ||
		selection.NativeContextLimit < MinInferenceContextTokens ||
		selection.NativeContextLimit > MaxInferenceContextTokens {
		return fmt.Errorf("provider identity discovery selection is invalid")
	}
	return nil
}

func RequireDiscoveredProviderIdentity(
	ctx context.Context,
	client Client,
	selection ProviderIdentitySelection,
) (ProviderIdentityAttestation, error) {
	if ctx == nil || client == nil {
		return ProviderIdentityAttestation{}, fmt.Errorf(
			"provider identity discovery requires context and client",
		)
	}
	if err := selection.Validate(); err != nil {
		return ProviderIdentityAttestation{}, err
	}
	discoverer, ok := client.(ProviderIdentityDiscoverer)
	if !ok {
		return ProviderIdentityAttestation{}, fmt.Errorf(
			"configured generation provider cannot discover its live identity",
		)
	}
	attestation, err := discoverer.DiscoverProviderIdentity(ctx, selection)
	if err != nil {
		return ProviderIdentityAttestation{}, err
	}
	if err := attestation.Validate(); err != nil {
		return ProviderIdentityAttestation{}, err
	}
	if attestation.Model != selection.Model ||
		attestation.NativeContextLimit != selection.NativeContextLimit {
		return ProviderIdentityAttestation{}, fmt.Errorf(
			"discovered provider identity changed the selected model or context allocation",
		)
	}
	return attestation, nil
}

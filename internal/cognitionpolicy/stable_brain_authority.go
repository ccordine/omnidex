package cognitionpolicy

import (
	"fmt"

	"github.com/gryph/omnidex/internal/llm"
)

const StableBrainAuthoritySchemaV1 = "omnidex.stable-brain-authority.v1"

type StableBrainAuthority struct {
	Schema                  string                          `json:"schema"`
	Ref                     BrainRef                        `json:"brain"`
	ProviderAttestation     llm.ProviderIdentityAttestation `json:"provider_attestation"`
	HostHardwareAttestation HostHardwareAttestation         `json:"host_hardware_attestation"`
	SHA256                  string                          `json:"sha256"`
}

func (brain AttestedBrain) StableAuthority() (StableBrainAuthority, error) {
	if err := brain.Validate(); err != nil {
		return StableBrainAuthority{}, err
	}
	value := StableBrainAuthority{
		Schema: StableBrainAuthoritySchemaV1, Ref: brain.Ref,
		ProviderAttestation: brain.Attestation, HostHardwareAttestation: brain.Host,
	}
	value.SHA256 = stableBrainAuthoritySHA(value)
	return value, value.Validate()
}

func (authority StableBrainAuthority) Validate() error {
	if authority.Schema != StableBrainAuthoritySchemaV1 ||
		!validPolicySHA256(authority.SHA256) ||
		authority.SHA256 != stableBrainAuthoritySHA(authority) {
		return fmt.Errorf("%w: stable brain authority identity is invalid", ErrInvalidBrain)
	}
	expected, err := authority.Ref.ProviderExpectation()
	if err != nil || authority.ProviderAttestation.ValidateFor(expected) != nil ||
		authority.HostHardwareAttestation.Validate() != nil {
		return fmt.Errorf("%w: stable brain authority evidence is invalid", ErrInvalidBrain)
	}
	return nil
}

func stableBrainAuthoritySHA(authority StableBrainAuthority) string {
	copy := authority
	copy.SHA256 = ""
	raw, err := canonicalPolicyJSON(copy)
	if err != nil {
		panic(fmt.Sprintf("marshal stable brain authority: %v", err))
	}
	return policySHA256(string(raw))
}

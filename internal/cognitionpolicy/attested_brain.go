package cognitionpolicy

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/llm"
)

type AttestedBrain struct {
	Ref         BrainRef                        `json:"brain"`
	Attestation llm.ProviderIdentityAttestation `json:"provider_attestation"`
	Host        HostHardwareAttestation         `json:"host_hardware_attestation"`
}

func AttestBrain(
	ctx context.Context,
	client llm.Client,
	brain BrainRef,
) (AttestedBrain, error) {
	expected, err := brain.ProviderExpectation()
	if err != nil {
		return AttestedBrain{}, err
	}
	attestation, err := llm.RequireProviderIdentityAttestation(ctx, client, expected)
	if err != nil {
		return AttestedBrain{}, fmt.Errorf("%w: live provider identity: %v", ErrInvalidBrain, err)
	}
	host, err := AttestLocalHostHardware()
	if err != nil {
		return AttestedBrain{}, fmt.Errorf("%w: live host hardware identity: %v", ErrInvalidBrain, err)
	}
	return NewAttestedBrain(brain, attestation, host)
}

func NewAttestedBrain(
	brain BrainRef,
	attestation llm.ProviderIdentityAttestation,
	host HostHardwareAttestation,
) (AttestedBrain, error) {
	value := AttestedBrain{Ref: brain, Attestation: attestation, Host: host}
	if err := value.Validate(); err != nil {
		return AttestedBrain{}, err
	}
	return value, nil
}

func (brain AttestedBrain) Validate() error {
	expected, err := brain.Ref.ProviderExpectation()
	if err != nil {
		return err
	}
	if err := brain.Attestation.ValidateFor(expected); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidBrain, err)
	}
	if err := brain.Host.Validate(); err != nil {
		return err
	}
	return nil
}

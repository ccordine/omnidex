package cognitionpolicy

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/llm"
)

type AttestedBrain struct {
	Ref                  BrainRef                        `json:"brain"`
	Attestation          llm.ProviderIdentityAttestation `json:"provider_attestation"`
	BootstrapObservation llm.ProviderIdentityObservation `json:"bootstrap_provider_observation"`
	Host                 HostHardwareAttestation         `json:"host_hardware_attestation"`
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
	if err := llm.ValidateExactPreparedProvider(client, expected); err != nil {
		return AttestedBrain{}, fmt.Errorf("%w: %v", ErrInvalidBrain, err)
	}
	request, err := BootstrapProviderIdentityRequest(brain)
	if err != nil {
		return AttestedBrain{}, err
	}
	observed, err := llm.RequireProviderIdentityObservation(ctx, client, request)
	if err != nil {
		return AttestedBrain{}, fmt.Errorf("%w: live provider identity: %v", ErrInvalidBrain, err)
	}
	host, err := AttestLocalHostHardware()
	if err != nil {
		return AttestedBrain{}, fmt.Errorf("%w: live host hardware identity: %v", ErrInvalidBrain, err)
	}
	return NewAttestedBrain(brain, observed.Attestation, observed.Observation, host)
}

func NewAttestedBrain(
	brain BrainRef,
	attestation llm.ProviderIdentityAttestation,
	observation llm.ProviderIdentityObservation,
	host HostHardwareAttestation,
) (AttestedBrain, error) {
	value := AttestedBrain{
		Ref: brain, Attestation: attestation,
		BootstrapObservation: observation, Host: host,
	}
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
	request, err := BootstrapProviderIdentityRequest(brain.Ref)
	if err != nil {
		return err
	}
	if err := brain.BootstrapObservation.ValidateFor(
		brain.Attestation, request.ChallengeSHA256,
	); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidBrain, err)
	}
	if err := brain.Host.Validate(); err != nil {
		return err
	}
	return nil
}

func BootstrapProviderIdentityRequest(
	brain BrainRef,
) (llm.ProviderIdentityObservationRequest, error) {
	expected, err := brain.ProviderExpectation()
	if err != nil {
		return llm.ProviderIdentityObservationRequest{}, err
	}
	challenge, err := llm.DeriveProviderIdentityObservationChallenge(
		"cognition-brain-bootstrap:"+brain.SamplingSHA256, expected,
	)
	if err != nil {
		return llm.ProviderIdentityObservationRequest{}, err
	}
	return llm.ProviderIdentityObservationRequest{
		Expectation: expected, ChallengeSHA256: challenge,
	}, nil
}

func ObserveAttestedBrainFresh(
	ctx context.Context,
	client llm.Client,
	brain AttestedBrain,
	scope string,
) (llm.ProviderIdentityObservation, error) {
	if err := brain.Validate(); err != nil {
		return llm.ProviderIdentityObservation{}, err
	}
	expected, err := brain.Ref.ProviderExpectation()
	if err != nil {
		return llm.ProviderIdentityObservation{}, err
	}
	challenge, err := llm.DeriveProviderIdentityObservationChallenge(scope, expected)
	if err != nil {
		return llm.ProviderIdentityObservation{}, err
	}
	observed, err := llm.RequireProviderIdentityObservation(
		ctx, client, llm.ProviderIdentityObservationRequest{
			Expectation: expected, ChallengeSHA256: challenge,
		},
	)
	if err != nil {
		return llm.ProviderIdentityObservation{}, err
	}
	if observed.Attestation != brain.Attestation {
		return llm.ProviderIdentityObservation{}, fmt.Errorf(
			"%w: fresh provider identity changed the frozen brain", ErrInvalidBrain,
		)
	}
	return observed.Observation, nil
}

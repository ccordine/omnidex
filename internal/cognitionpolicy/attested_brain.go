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
) (BrainBootstrapOutcome, error) {
	return attestBrainWithHostAttestor(ctx, client, brain, AttestLocalHostHardware)
}

func attestBrainWithHostAttestor(
	ctx context.Context,
	client llm.Client,
	brain BrainRef,
	hostAttestor func() (HostHardwareAttestation, error),
) (BrainBootstrapOutcome, error) {
	if hostAttestor == nil {
		return BrainBootstrapOutcome{}, fmt.Errorf("%w: host hardware attestor is nil", ErrInvalidBrain)
	}
	expected, err := brain.ProviderExpectation()
	if err != nil {
		return BrainBootstrapOutcome{}, err
	}
	if err := llm.ValidateExactPreparedProvider(client, expected); err != nil {
		return BrainBootstrapOutcome{}, fmt.Errorf("%w: %v", ErrInvalidBrain, err)
	}
	request, err := BootstrapProviderIdentityRequest(brain)
	if err != nil {
		return BrainBootstrapOutcome{}, err
	}
	observed, err := llm.RequireProviderIdentityObservation(ctx, client, request)
	if err != nil {
		code, codeErr := providerIdentityFailureCodeForObserved(brain, request, observed)
		if codeErr != nil {
			return BrainBootstrapOutcome{}, fmt.Errorf(
				"%w: live provider identity returned unrecordable evidence: %v",
				ErrInvalidBrain, codeErr,
			)
		}
		outcome, outcomeErr := newBrainBootstrapFailure(brain, request, observed, code)
		if outcomeErr != nil {
			return BrainBootstrapOutcome{}, outcomeErr
		}
		return outcome, fmt.Errorf("%w: live provider identity: %v", ErrInvalidBrain, err)
	}
	host, err := hostAttestor()
	if err != nil {
		outcome, outcomeErr := newBrainBootstrapFailure(
			brain, request, observed, ProviderHostAttestationFailed,
		)
		if outcomeErr != nil {
			return BrainBootstrapOutcome{}, outcomeErr
		}
		return outcome, fmt.Errorf("%w: live host hardware identity: %v", ErrInvalidBrain, err)
	}
	attested, err := NewAttestedBrain(brain, observed.Attestation, observed.Observation, host)
	if err != nil {
		return BrainBootstrapOutcome{}, err
	}
	bootstrap, err := NewBrainBootstrap(attested, observed.Evidence)
	if err != nil {
		return BrainBootstrapOutcome{}, err
	}
	return newSuccessfulBrainBootstrapOutcome(bootstrap)
}

func providerIdentityFailureCodeForObserved(
	brain BrainRef,
	request llm.ProviderIdentityObservationRequest,
	observed llm.ObservedProviderIdentity,
) (ProviderIdentityFailureCode, error) {
	selection := llm.ProviderIdentitySelection{
		Model: brain.Model, NativeContextLimit: brain.NativeContextLimit,
	}
	expected, err := brain.ProviderExpectation()
	if err != nil {
		return "", err
	}
	if observed.Evidence.ValidateFailure(selection, &expected) == nil {
		return ProviderIdentityObservationFailed, nil
	}
	if observed.Evidence.ValidateRequests(selection) == nil && observed.Evidence.Successful() {
		if !providerIdentityFailureProofBounded(providerIdentityFailureProof{
			Attestation: observed.Attestation, Observation: observed.Observation,
		}) {
			return "", fmt.Errorf("provider identity normalized metadata exceeds its recordable bound")
		}
		if observed.ValidateFor(request) != nil {
			return ProviderIdentityObservationInvalid, nil
		}
	}
	return "", fmt.Errorf("bounded raw evidence does not prove the returned failure")
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

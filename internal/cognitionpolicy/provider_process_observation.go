package cognitionpolicy

import (
	"context"
	"fmt"
	"regexp"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/llm"
)

const ProviderProcessObservationSchemaV1 = "omnidex.provider-process-observation.v1"

var providerProcessObservationIDPattern = regexp.MustCompile(
	`^provider_process_observation_[0-9a-f]{64}$`,
)

type ProviderProcessObservationPurpose string

const ProviderProcessEpisodeInvocation ProviderProcessObservationPurpose = "episode_invocation"

type ProviderProcessObservation struct {
	Schema      string                            `json:"schema"`
	ID          string                            `json:"id"`
	EpisodeID   cognition.EpisodeID               `json:"episode_id"`
	Actor       cognition.AttemptRef              `json:"actor"`
	Purpose     ProviderProcessObservationPurpose `json:"purpose"`
	StableBrain StableBrainAuthority              `json:"stable_brain"`
	Observation llm.ProviderIdentityObservation   `json:"observation"`
}

func ObserveProviderProcess(
	ctx context.Context,
	client llm.Client,
	brain AttestedBrain,
	episode cognition.EpisodeRef,
	actor cognition.AttemptRef,
	purpose ProviderProcessObservationPurpose,
) (ProviderProcessOutcome, error) {
	return observeProviderProcessWithHostAttestor(
		ctx, client, brain, episode, actor, purpose, AttestLocalHostHardware,
	)
}

func observeProviderProcessWithHostAttestor(
	ctx context.Context,
	client llm.Client,
	brain AttestedBrain,
	episode cognition.EpisodeRef,
	actor cognition.AttemptRef,
	purpose ProviderProcessObservationPurpose,
	hostAttestor func() (HostHardwareAttestation, error),
) (ProviderProcessOutcome, error) {
	if hostAttestor == nil {
		return ProviderProcessOutcome{}, fmt.Errorf("%w: host hardware attestor is nil", ErrInvalidBrain)
	}
	request, stable, err := providerProcessObservationRequest(brain, episode, actor, purpose)
	if err != nil {
		return ProviderProcessOutcome{}, err
	}
	observed, err := llm.RequireProviderIdentityObservation(ctx, client, request)
	if err != nil {
		code, codeErr := providerIdentityFailureCodeForObserved(brain.Ref, request, observed)
		if codeErr != nil {
			return ProviderProcessOutcome{}, fmt.Errorf(
				"%w: process provider identity returned unrecordable evidence: %v",
				ErrInvalidEvidence, codeErr,
			)
		}
		outcome, outcomeErr := newProviderProcessFailure(
			brain, episode, actor, purpose, observed, HostHardwareAttestation{}, code,
		)
		if outcomeErr != nil {
			return ProviderProcessOutcome{}, outcomeErr
		}
		return outcome, err
	}
	if observed.Attestation != brain.Attestation {
		outcome, outcomeErr := newProviderProcessFailure(
			brain, episode, actor, purpose, observed, HostHardwareAttestation{},
			ProviderAttestationIdentityMismatch,
		)
		if outcomeErr != nil {
			return ProviderProcessOutcome{}, outcomeErr
		}
		return outcome, fmt.Errorf(
			"%w: process observation changed the stable provider authority", ErrInvalidBrain,
		)
	}
	liveHost, err := hostAttestor()
	if err != nil {
		outcome, outcomeErr := newProviderProcessFailure(
			brain, episode, actor, purpose, observed, HostHardwareAttestation{},
			ProviderHostAttestationFailed,
		)
		if outcomeErr != nil {
			return ProviderProcessOutcome{}, outcomeErr
		}
		return outcome, fmt.Errorf("%w: process host observation: %v", ErrInvalidBrain, err)
	}
	if liveHost != brain.Host {
		outcome, outcomeErr := newProviderProcessFailure(
			brain, episode, actor, purpose, observed, liveHost,
			ProviderHostIdentityMismatch,
		)
		if outcomeErr != nil {
			return ProviderProcessOutcome{}, outcomeErr
		}
		return outcome, fmt.Errorf(
			"%w: process host differs from the stored Brain", ErrInvalidBrain,
		)
	}
	receipt := ProviderProcessObservation{
		Schema: ProviderProcessObservationSchemaV1, EpisodeID: episode.ID,
		Actor: actor, Purpose: purpose, StableBrain: stable,
		Observation: observed.Observation,
	}
	receipt.ID = providerProcessObservationID(receipt)
	activation, err := NewProviderProcessActivation(receipt, observed.Evidence, brain)
	if err != nil {
		return ProviderProcessOutcome{}, err
	}
	return newSuccessfulProviderProcessOutcome(activation)
}

func (receipt ProviderProcessObservation) ValidateFor(brain AttestedBrain) error {
	if receipt.Schema != ProviderProcessObservationSchemaV1 ||
		receipt.ID != providerProcessObservationID(receipt) ||
		ProviderProcessObservationPurpose(receipt.Purpose) != ProviderProcessEpisodeInvocation {
		return fmt.Errorf("%w: provider process observation identity is invalid", ErrInvalidEvidence)
	}
	episode, err := cognition.NewEpisodeRef(receipt.EpisodeID)
	if err != nil {
		return err
	}
	request, stable, err := providerProcessObservationRequest(
		brain, episode, receipt.Actor, receipt.Purpose,
	)
	if err != nil {
		return err
	}
	if receipt.StableBrain != stable ||
		receipt.Observation.ValidateFor(brain.Attestation, request.ChallengeSHA256) != nil {
		return fmt.Errorf("%w: provider process observation changed its authority", ErrInvalidEvidence)
	}
	return nil
}

func providerProcessObservationRequest(
	brain AttestedBrain,
	episode cognition.EpisodeRef,
	actor cognition.AttemptRef,
	purpose ProviderProcessObservationPurpose,
) (llm.ProviderIdentityObservationRequest, StableBrainAuthority, error) {
	if err := brain.Validate(); err != nil {
		return llm.ProviderIdentityObservationRequest{}, StableBrainAuthority{}, err
	}
	if err := episode.Validate(); err != nil {
		return llm.ProviderIdentityObservationRequest{}, StableBrainAuthority{}, err
	}
	if err := actor.Validate(); err != nil {
		return llm.ProviderIdentityObservationRequest{}, StableBrainAuthority{}, err
	}
	if purpose != ProviderProcessEpisodeInvocation {
		return llm.ProviderIdentityObservationRequest{}, StableBrainAuthority{},
			fmt.Errorf("provider process observation purpose is not registered")
	}
	stable, err := brain.StableAuthority()
	if err != nil {
		return llm.ProviderIdentityObservationRequest{}, StableBrainAuthority{}, err
	}
	request, err := providerProcessObservationRequestForStable(
		stable, episode, actor, purpose,
	)
	return request, stable, err
}

func providerProcessObservationRequestForStable(
	stable StableBrainAuthority,
	episode cognition.EpisodeRef,
	actor cognition.AttemptRef,
	purpose ProviderProcessObservationPurpose,
) (llm.ProviderIdentityObservationRequest, error) {
	if err := stable.Validate(); err != nil {
		return llm.ProviderIdentityObservationRequest{}, err
	}
	if err := episode.Validate(); err != nil {
		return llm.ProviderIdentityObservationRequest{}, err
	}
	if err := actor.Validate(); err != nil {
		return llm.ProviderIdentityObservationRequest{}, err
	}
	if purpose != ProviderProcessEpisodeInvocation {
		return llm.ProviderIdentityObservationRequest{},
			fmt.Errorf("provider process observation purpose is not registered")
	}
	scopeRaw, err := canonicalPolicyJSON(struct {
		EpisodeID cognition.EpisodeID               `json:"episode_id"`
		Actor     cognition.AttemptRef              `json:"actor"`
		Purpose   ProviderProcessObservationPurpose `json:"purpose"`
		BrainSHA  string                            `json:"stable_brain_sha256"`
	}{episode.ID, actor, purpose, stable.SHA256})
	if err != nil {
		return llm.ProviderIdentityObservationRequest{}, err
	}
	expected, err := stable.Ref.ProviderExpectation()
	if err != nil {
		return llm.ProviderIdentityObservationRequest{}, err
	}
	challenge, err := llm.DeriveProviderIdentityObservationChallenge(
		"cognition-process:"+policySHA256(string(scopeRaw)), expected,
	)
	return llm.ProviderIdentityObservationRequest{
		Expectation: expected, ChallengeSHA256: challenge,
	}, err
}

func providerProcessObservationID(receipt ProviderProcessObservation) string {
	copy := receipt
	copy.ID = ""
	raw, err := canonicalPolicyJSON(copy)
	if err != nil {
		panic(fmt.Sprintf("marshal provider process observation: %v", err))
	}
	return "provider_process_observation_" + policySHA256(string(raw))
}

package cognitionpolicy

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/llm"
)

const ProviderProcessObservationSchemaV1 = "omnidex.provider-process-observation.v1"

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
) (ProviderProcessObservation, error) {
	request, stable, err := providerProcessObservationRequest(brain, episode, actor, purpose)
	if err != nil {
		return ProviderProcessObservation{}, err
	}
	observed, err := llm.RequireProviderIdentityObservation(ctx, client, request)
	if err != nil {
		return ProviderProcessObservation{}, err
	}
	if observed.Attestation != brain.Attestation {
		return ProviderProcessObservation{}, fmt.Errorf(
			"%w: process observation changed the stable provider authority", ErrInvalidBrain,
		)
	}
	receipt := ProviderProcessObservation{
		Schema: ProviderProcessObservationSchemaV1, EpisodeID: episode.ID,
		Actor: actor, Purpose: purpose, StableBrain: stable,
		Observation: observed.Observation,
	}
	receipt.ID = providerProcessObservationID(receipt)
	return receipt, receipt.ValidateFor(brain)
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
	scopeRaw, err := canonicalPolicyJSON(struct {
		EpisodeID cognition.EpisodeID               `json:"episode_id"`
		Actor     cognition.AttemptRef              `json:"actor"`
		Purpose   ProviderProcessObservationPurpose `json:"purpose"`
		BrainSHA  string                            `json:"stable_brain_sha256"`
	}{episode.ID, actor, purpose, stable.SHA256})
	if err != nil {
		return llm.ProviderIdentityObservationRequest{}, StableBrainAuthority{}, err
	}
	expected, err := brain.Ref.ProviderExpectation()
	if err != nil {
		return llm.ProviderIdentityObservationRequest{}, StableBrainAuthority{}, err
	}
	challenge, err := llm.DeriveProviderIdentityObservationChallenge(
		"cognition-process:"+policySHA256(string(scopeRaw)), expected,
	)
	return llm.ProviderIdentityObservationRequest{
		Expectation: expected, ChallengeSHA256: challenge,
	}, stable, err
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

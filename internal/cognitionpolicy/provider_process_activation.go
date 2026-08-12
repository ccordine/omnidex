package cognitionpolicy

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/llm"
)

const ProviderProcessActivationAuthoritySchemaV1 = "omnidex.provider-process-activation-authority.v1"

// ProviderProcessActivationAuthority is the exact durable provider-process
// observation a policy invocation is permitted to consume. It is carried into
// every CallAttempt so a call cannot borrow another invocation's observation.
type ProviderProcessActivationAuthority struct {
	Schema                    string                          `json:"schema"`
	ObservationID             string                          `json:"observation_id"`
	EpisodeID                 cognition.EpisodeID             `json:"episode_id"`
	Actor                     cognition.AttemptRef            `json:"actor"`
	StableBrainSHA256         string                          `json:"stable_brain_sha256"`
	ProviderObservationSHA256 string                          `json:"provider_observation_sha256"`
	Evidence                  llm.ProviderIdentityEvidenceRef `json:"evidence"`
}

// ProviderProcessActivation keeps the normalized process receipt indivisible
// from the exact provider request and response bodies that produced it.
type ProviderProcessActivation struct {
	Receipt          ProviderProcessObservation
	IdentityEvidence llm.ProviderIdentityEvidence
}

func NewProviderProcessActivation(
	receipt ProviderProcessObservation,
	evidence llm.ProviderIdentityEvidence,
	brain AttestedBrain,
) (ProviderProcessActivation, error) {
	owned, err := llm.OwnBoundedProviderIdentityEvidence(evidence)
	if err != nil {
		return ProviderProcessActivation{}, fmt.Errorf("%w: %v", ErrInvalidEvidence, err)
	}
	value := ProviderProcessActivation{
		Receipt: receipt, IdentityEvidence: owned,
	}
	if err := value.ValidateFor(brain); err != nil {
		return ProviderProcessActivation{}, err
	}
	return value, nil
}

func (activation ProviderProcessActivation) ValidateFor(brain AttestedBrain) error {
	if err := activation.Receipt.ValidateFor(brain); err != nil {
		return err
	}
	selection := llm.ProviderIdentitySelection{
		Model: brain.Ref.Model, NativeContextLimit: brain.Ref.NativeContextLimit,
	}
	if activation.IdentityEvidence.ValidateRequests(selection) != nil ||
		!activation.IdentityEvidence.Successful() ||
		activation.Receipt.Observation.ValidateEvidence(activation.IdentityEvidence) != nil {
		return fmt.Errorf("%w: provider process receipt lacks exact raw identity evidence", ErrInvalidEvidence)
	}
	return nil
}

func (activation ProviderProcessActivation) Authority() (
	ProviderProcessActivationAuthority,
	error,
) {
	if err := activation.ValidateForStableReceipt(); err != nil {
		return ProviderProcessActivationAuthority{}, err
	}
	value := ProviderProcessActivationAuthority{
		Schema:        ProviderProcessActivationAuthoritySchemaV1,
		ObservationID: activation.Receipt.ID, EpisodeID: activation.Receipt.EpisodeID,
		Actor:                     activation.Receipt.Actor,
		StableBrainSHA256:         activation.Receipt.StableBrain.SHA256,
		ProviderObservationSHA256: activation.Receipt.Observation.ObservationSHA256,
		Evidence:                  activation.IdentityEvidence.Ref,
	}
	return value, value.Validate()
}

func (activation ProviderProcessActivation) ValidateForStableReceipt() error {
	if activation.Receipt.Schema != ProviderProcessObservationSchemaV1 ||
		activation.Receipt.ID != providerProcessObservationID(activation.Receipt) ||
		activation.IdentityEvidence.Validate() != nil ||
		activation.Receipt.Observation.ValidateEvidence(activation.IdentityEvidence) != nil {
		return fmt.Errorf("%w: provider process activation is not an exact receipt", ErrInvalidEvidence)
	}
	return nil
}

func (authority ProviderProcessActivationAuthority) Validate() error {
	if authority.Schema != ProviderProcessActivationAuthoritySchemaV1 ||
		!providerProcessObservationIDPattern.MatchString(authority.ObservationID) ||
		authority.EpisodeID == "" || authority.Actor.Validate() != nil ||
		!validPolicySHA256(authority.StableBrainSHA256) ||
		!validPolicySHA256(authority.ProviderObservationSHA256) ||
		authority.Evidence.Validate() != nil {
		return fmt.Errorf("%w: provider process activation authority is invalid", ErrInvalidEvidence)
	}
	return nil
}

func (authority ProviderProcessActivationAuthority) ValidateFor(
	brain StableBrainAuthority,
	episode cognition.EpisodeID,
	actor cognition.AttemptRef,
) error {
	if err := authority.Validate(); err != nil {
		return err
	}
	if err := brain.Validate(); err != nil {
		return err
	}
	if authority.EpisodeID != episode || authority.Actor != actor ||
		authority.StableBrainSHA256 != brain.SHA256 {
		return fmt.Errorf("%w: provider process activation differs from the call authority", ErrInvalidEvidence)
	}
	return nil
}

func (activation ProviderProcessActivation) Clone() ProviderProcessActivation {
	activation.IdentityEvidence = activation.IdentityEvidence.Clone()
	return activation
}

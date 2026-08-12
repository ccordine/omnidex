package cognitionpolicy

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/llm"
)

const ProviderProcessFailureSchemaV1 = "omnidex.provider-process-failure.v1"

type ProviderProcessFailureReceipt struct {
	Schema              string                            `json:"schema"`
	ID                  string                            `json:"id"`
	EpisodeID           cognition.EpisodeID               `json:"episode_id"`
	Actor               cognition.AttemptRef              `json:"actor"`
	Purpose             ProviderProcessObservationPurpose `json:"purpose"`
	StableBrain         StableBrainAuthority              `json:"stable_brain"`
	Code                ProviderIdentityFailureCode       `json:"code"`
	ProviderAttestation llm.ProviderIdentityAttestation   `json:"provider_attestation"`
	ProviderObservation llm.ProviderIdentityObservation   `json:"provider_observation"`
	LiveHost            HostHardwareAttestation           `json:"live_host_hardware_attestation"`
	Evidence            llm.ProviderIdentityEvidenceRef   `json:"evidence"`
}

type ProviderProcessFailure struct {
	Receipt          ProviderProcessFailureReceipt
	IdentityEvidence llm.ProviderIdentityEvidence
}

type ProviderProcessOutcome struct {
	Success *ProviderProcessActivation
	Failure *ProviderProcessFailure
}

func newProviderProcessFailure(
	brain AttestedBrain,
	episode cognition.EpisodeRef,
	actor cognition.AttemptRef,
	purpose ProviderProcessObservationPurpose,
	observed llm.ObservedProviderIdentity,
	liveHost HostHardwareAttestation,
	code ProviderIdentityFailureCode,
) (ProviderProcessOutcome, error) {
	request, stable, err := providerProcessObservationRequest(
		brain, episode, actor, purpose,
	)
	if err != nil {
		return ProviderProcessOutcome{}, err
	}
	owned, err := llm.OwnBoundedProviderIdentityEvidence(observed.Evidence)
	if err != nil {
		return ProviderProcessOutcome{}, err
	}
	proof := providerIdentityFailureProof{
		Attestation: observed.Attestation, Observation: observed.Observation,
	}
	if !providerIdentityFailureProofBounded(proof) {
		return ProviderProcessOutcome{}, fmt.Errorf(
			"%w: provider identity normalized metadata exceeds its recordable bound",
			ErrInvalidEvidence,
		)
	}
	receipt := ProviderProcessFailureReceipt{
		Schema: ProviderProcessFailureSchemaV1, EpisodeID: episode.ID,
		Actor: actor, Purpose: purpose, StableBrain: stable, Code: code,
		ProviderAttestation: proof.Attestation, ProviderObservation: proof.Observation,
		LiveHost: liveHost, Evidence: owned.Ref,
	}
	receipt.ID = providerProcessFailureID(receipt)
	failure := ProviderProcessFailure{Receipt: receipt, IdentityEvidence: owned}
	if err := failure.ValidateFor(brain); err != nil {
		return ProviderProcessOutcome{}, err
	}
	_ = request
	return ProviderProcessOutcome{Failure: &failure}, nil
}

func newSuccessfulProviderProcessOutcome(
	activation ProviderProcessActivation,
) (ProviderProcessOutcome, error) {
	if err := activation.ValidateForStableReceipt(); err != nil {
		return ProviderProcessOutcome{}, err
	}
	copy := activation.Clone()
	return ProviderProcessOutcome{Success: &copy}, nil
}

func (outcome ProviderProcessOutcome) ValidateFor(brain AttestedBrain) error {
	if (outcome.Success == nil) == (outcome.Failure == nil) {
		return fmt.Errorf("%w: process outcome must contain exactly one result", ErrInvalidEvidence)
	}
	if outcome.Success != nil {
		return outcome.Success.ValidateFor(brain)
	}
	return outcome.Failure.ValidateFor(brain)
}

func (outcome ProviderProcessOutcome) RequireSuccess(
	brain AttestedBrain,
) (ProviderProcessActivation, error) {
	if err := outcome.ValidateFor(brain); err != nil {
		return ProviderProcessActivation{}, err
	}
	if outcome.Success == nil {
		return ProviderProcessActivation{}, fmt.Errorf(
			"%w: provider process activation ended in a recorded failure", ErrInvalidEvidence,
		)
	}
	return outcome.Success.Clone(), nil
}

func (failure ProviderProcessFailure) ValidateFor(brain AttestedBrain) error {
	stable, err := brain.StableAuthority()
	if err != nil {
		return err
	}
	if failure.Receipt.StableBrain != stable {
		return fmt.Errorf("%w: process failure changed the stored Brain", ErrInvalidEvidence)
	}
	return failure.ValidateForStableBrain()
}

func (failure ProviderProcessFailure) ValidateForStableBrain() error {
	receipt := failure.Receipt
	episode := cognition.EpisodeRef{ID: receipt.EpisodeID}
	request, err := providerProcessObservationRequestForStable(
		receipt.StableBrain, episode, receipt.Actor, receipt.Purpose,
	)
	if err != nil {
		return err
	}
	if receipt.Schema != ProviderProcessFailureSchemaV1 ||
		receipt.ID != providerProcessFailureID(receipt) ||
		receipt.Evidence != failure.IdentityEvidence.Ref {
		return fmt.Errorf("%w: provider process failure identity is invalid", ErrInvalidEvidence)
	}
	if err := validateProviderIdentityFailureProof(
		receipt.Code, receipt.StableBrain.Ref, request.ChallengeSHA256,
		providerIdentityFailureProof{
			Attestation: receipt.ProviderAttestation,
			Observation: receipt.ProviderObservation,
		}, failure.IdentityEvidence,
	); err != nil {
		return err
	}
	switch receipt.Code {
	case ProviderAttestationIdentityMismatch:
		if receipt.ProviderAttestation == receipt.StableBrain.ProviderAttestation ||
			receipt.LiveHost != (HostHardwareAttestation{}) {
			return fmt.Errorf("%w: provider attestation mismatch is not proven", ErrInvalidEvidence)
		}
	case ProviderHostAttestationFailed:
		if receipt.ProviderAttestation != receipt.StableBrain.ProviderAttestation ||
			receipt.LiveHost != (HostHardwareAttestation{}) {
			return fmt.Errorf("%w: failed host probe claims a live host", ErrInvalidEvidence)
		}
	case ProviderHostIdentityMismatch:
		if receipt.ProviderAttestation != receipt.StableBrain.ProviderAttestation ||
			receipt.LiveHost.Validate() != nil ||
			receipt.LiveHost == receipt.StableBrain.HostHardwareAttestation {
			return fmt.Errorf("%w: host mismatch failure is not proven", ErrInvalidEvidence)
		}
	default:
		if receipt.LiveHost != (HostHardwareAttestation{}) {
			return fmt.Errorf("%w: provider failure claims unrelated host evidence", ErrInvalidEvidence)
		}
	}
	return nil
}

func (failure ProviderProcessFailure) Clone() ProviderProcessFailure {
	failure.IdentityEvidence = failure.IdentityEvidence.Clone()
	return failure
}

func providerProcessFailureID(receipt ProviderProcessFailureReceipt) string {
	receipt.ID = ""
	raw, err := canonicalPolicyJSON(receipt)
	if err != nil {
		panic(fmt.Sprintf("marshal provider process failure: %v", err))
	}
	return "provider_process_failure_" + policySHA256(string(raw))
}

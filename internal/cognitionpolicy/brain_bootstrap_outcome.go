package cognitionpolicy

import (
	"fmt"

	"github.com/gryph/omnidex/internal/llm"
)

const BrainBootstrapFailureSchemaV1 = "omnidex.brain-bootstrap-failure.v1"

type BrainBootstrapFailureReceipt struct {
	Schema              string                          `json:"schema"`
	ID                  string                          `json:"id"`
	Brain               BrainRef                        `json:"brain"`
	ChallengeSHA256     string                          `json:"challenge_sha256"`
	Code                ProviderIdentityFailureCode     `json:"code"`
	ProviderAttestation llm.ProviderIdentityAttestation `json:"provider_attestation"`
	ProviderObservation llm.ProviderIdentityObservation `json:"provider_observation"`
	Evidence            llm.ProviderIdentityEvidenceRef `json:"evidence"`
}

type BrainBootstrapFailure struct {
	Receipt          BrainBootstrapFailureReceipt
	IdentityEvidence llm.ProviderIdentityEvidence
}

type BrainBootstrapOutcome struct {
	Success *BrainBootstrap
	Failure *BrainBootstrapFailure
}

func newBrainBootstrapFailure(
	brain BrainRef,
	request llm.ProviderIdentityObservationRequest,
	observed llm.ObservedProviderIdentity,
	code ProviderIdentityFailureCode,
) (BrainBootstrapOutcome, error) {
	owned, err := llm.OwnBoundedProviderIdentityEvidence(observed.Evidence)
	if err != nil {
		return BrainBootstrapOutcome{}, err
	}
	proof := providerIdentityFailureProof{
		Attestation: observed.Attestation, Observation: observed.Observation,
	}
	if !providerIdentityFailureProofBounded(proof) {
		return BrainBootstrapOutcome{}, fmt.Errorf(
			"%w: provider identity normalized metadata exceeds its recordable bound",
			ErrInvalidEvidence,
		)
	}
	receipt := BrainBootstrapFailureReceipt{
		Schema: BrainBootstrapFailureSchemaV1, Brain: brain,
		ChallengeSHA256: request.ChallengeSHA256, Code: code,
		ProviderAttestation: proof.Attestation, ProviderObservation: proof.Observation,
		Evidence: owned.Ref,
	}
	receipt.ID = brainBootstrapFailureID(receipt)
	failure := BrainBootstrapFailure{Receipt: receipt, IdentityEvidence: owned}
	if err := failure.Validate(); err != nil {
		return BrainBootstrapOutcome{}, err
	}
	return BrainBootstrapOutcome{Failure: &failure}, nil
}

func newSuccessfulBrainBootstrapOutcome(
	bootstrap BrainBootstrap,
) (BrainBootstrapOutcome, error) {
	if err := bootstrap.Validate(); err != nil {
		return BrainBootstrapOutcome{}, err
	}
	copy := bootstrap.Clone()
	return BrainBootstrapOutcome{Success: &copy}, nil
}

func (outcome BrainBootstrapOutcome) Validate() error {
	if (outcome.Success == nil) == (outcome.Failure == nil) {
		return fmt.Errorf("%w: bootstrap outcome must contain exactly one result", ErrInvalidEvidence)
	}
	if outcome.Success != nil {
		return outcome.Success.Validate()
	}
	return outcome.Failure.Validate()
}

func (outcome BrainBootstrapOutcome) RequireSuccess() (BrainBootstrap, error) {
	if err := outcome.Validate(); err != nil {
		return BrainBootstrap{}, err
	}
	if outcome.Success == nil {
		return BrainBootstrap{}, fmt.Errorf("%w: Brain bootstrap ended in a recorded failure", ErrInvalidBrain)
	}
	return outcome.Success.Clone(), nil
}

func (failure BrainBootstrapFailure) Validate() error {
	receipt := failure.Receipt
	if receipt.Code == ProviderHostIdentityMismatch ||
		receipt.Code == ProviderAttestationIdentityMismatch {
		return fmt.Errorf("%w: bootstrap failure cannot claim a stored-host mismatch", ErrInvalidEvidence)
	}
	request, err := BootstrapProviderIdentityRequest(receipt.Brain)
	if err != nil {
		return err
	}
	if receipt.Schema != BrainBootstrapFailureSchemaV1 ||
		receipt.ID != brainBootstrapFailureID(receipt) ||
		receipt.ChallengeSHA256 != request.ChallengeSHA256 ||
		receipt.Evidence != failure.IdentityEvidence.Ref {
		return fmt.Errorf("%w: bootstrap failure identity is invalid", ErrInvalidEvidence)
	}
	return validateProviderIdentityFailureProof(
		receipt.Code, receipt.Brain, receipt.ChallengeSHA256,
		providerIdentityFailureProof{
			Attestation: receipt.ProviderAttestation,
			Observation: receipt.ProviderObservation,
		}, failure.IdentityEvidence,
	)
}

func (failure BrainBootstrapFailure) Clone() BrainBootstrapFailure {
	failure.IdentityEvidence = failure.IdentityEvidence.Clone()
	return failure
}

func brainBootstrapFailureID(receipt BrainBootstrapFailureReceipt) string {
	receipt.ID = ""
	raw, err := canonicalPolicyJSON(receipt)
	if err != nil {
		panic(fmt.Sprintf("marshal Brain bootstrap failure: %v", err))
	}
	return "brain_bootstrap_failure_" + policySHA256(string(raw))
}

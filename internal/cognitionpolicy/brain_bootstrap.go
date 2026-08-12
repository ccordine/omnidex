package cognitionpolicy

import (
	"fmt"

	"github.com/gryph/omnidex/internal/llm"
)

// BrainBootstrap is the indivisible output of one live provider and host
// attestation. The normalized Brain cannot escape without the exact raw
// provider operations that prove its bootstrap observation.
type BrainBootstrap struct {
	AttestedBrain     AttestedBrain
	BootstrapEvidence llm.ProviderIdentityEvidence
}

func NewBrainBootstrap(
	brain AttestedBrain,
	evidence llm.ProviderIdentityEvidence,
) (BrainBootstrap, error) {
	owned, err := llm.OwnBoundedProviderIdentityEvidence(evidence)
	if err != nil {
		return BrainBootstrap{}, fmt.Errorf("%w: %v", ErrInvalidBrain, err)
	}
	value := BrainBootstrap{AttestedBrain: brain, BootstrapEvidence: owned}
	if err := value.Validate(); err != nil {
		return BrainBootstrap{}, err
	}
	return value, nil
}

func (bootstrap BrainBootstrap) Validate() error {
	if err := bootstrap.AttestedBrain.Validate(); err != nil {
		return err
	}
	selection := llm.ProviderIdentitySelection{
		Model:              bootstrap.AttestedBrain.Ref.Model,
		NativeContextLimit: bootstrap.AttestedBrain.Ref.NativeContextLimit,
	}
	if bootstrap.BootstrapEvidence.ValidateRequests(selection) != nil ||
		!bootstrap.BootstrapEvidence.Successful() ||
		bootstrap.AttestedBrain.BootstrapObservation.ValidateEvidence(
			bootstrap.BootstrapEvidence,
		) != nil {
		return fmt.Errorf("%w: bootstrap Brain lacks its exact raw provider evidence", ErrInvalidBrain)
	}
	return nil
}

func (bootstrap BrainBootstrap) Clone() BrainBootstrap {
	bootstrap.BootstrapEvidence = bootstrap.BootstrapEvidence.Clone()
	return bootstrap
}

package cognitiongauntlet

import (
	"fmt"
	"path/filepath"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
)

func (artifact runtimeProviderActivationEvidenceArtifact) verifyFor(
	frozen BrainFingerprint,
) (cognitionpolicy.ProviderProcessActivation, error) {
	if artifact.Schema != RuntimeProviderActivationEvidenceArtifactSchemaV1 {
		return cognitionpolicy.ProviderProcessActivation{}, fmt.Errorf("runtime provider activation artifact identity is invalid")
	}
	brain, err := frozen.attestedBrain()
	if err != nil {
		return cognitionpolicy.ProviderProcessActivation{}, err
	}
	observed, err := artifact.Capture.observed()
	if err != nil {
		return cognitionpolicy.ProviderProcessActivation{}, fmt.Errorf("reconstruct runtime provider activation evidence: %w", err)
	}
	if observed.Attestation != brain.Attestation ||
		observed.Observation != artifact.Receipt.Observation {
		return cognitionpolicy.ProviderProcessActivation{}, fmt.Errorf("runtime provider activation receipt differs from its raw evidence")
	}
	activation, err := cognitionpolicy.NewProviderProcessActivation(
		artifact.Receipt, observed.Evidence, brain,
	)
	if err != nil {
		return cognitionpolicy.ProviderProcessActivation{}, fmt.Errorf("verify runtime provider activation evidence: %w", err)
	}
	return activation, nil
}

func (authority RuntimeProviderActivationEvidenceAuthority) Validate() error {
	if authority.Schema != RuntimeProviderActivationEvidenceAuthoritySchemaV1 ||
		authority.ID != "runtime_provider_activation_evidence_"+authority.SHA256 ||
		!validDigest(authority.SHA256) || authority.Bytes < 1 ||
		authority.Bytes > maxRuntimeProviderActivationEvidenceBytes ||
		requireExact(authority.ObservationID, "runtime provider observation ID", 512) != nil ||
		authority.Evidence.Validate() != nil ||
		authority.File != "runtime-provider-activation-"+authority.SHA256+".json" ||
		filepath.Base(authority.File) != authority.File {
		return fmt.Errorf("runtime provider activation evidence authority is invalid")
	}
	return nil
}

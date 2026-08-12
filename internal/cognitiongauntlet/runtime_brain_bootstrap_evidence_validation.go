package cognitiongauntlet

import (
	"fmt"
	"path/filepath"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
)

func (artifact runtimeBrainBootstrapEvidenceArtifact) verifyFor(
	frozen BrainFingerprint,
) (cognitionpolicy.BrainBootstrap, error) {
	if artifact.Schema != RuntimeBrainBootstrapEvidenceArtifactSchemaV1 {
		return cognitionpolicy.BrainBootstrap{}, fmt.Errorf("runtime Brain bootstrap artifact identity is invalid")
	}
	if err := frozen.Validate(); err != nil {
		return cognitionpolicy.BrainBootstrap{}, err
	}
	observed, err := artifact.Capture.observed()
	if err != nil {
		return cognitionpolicy.BrainBootstrap{}, fmt.Errorf("reconstruct runtime Brain bootstrap evidence: %w", err)
	}
	if observed.Attestation != artifact.Brain.Attestation ||
		observed.Observation != artifact.Brain.BootstrapObservation {
		return cognitionpolicy.BrainBootstrap{}, fmt.Errorf("runtime Brain bootstrap normalized identity differs from its raw evidence")
	}
	bootstrap, err := cognitionpolicy.NewBrainBootstrap(artifact.Brain, observed.Evidence)
	if err != nil {
		return cognitionpolicy.BrainBootstrap{}, fmt.Errorf("verify runtime Brain bootstrap evidence: %w", err)
	}
	want, err := frozen.attestedBrain()
	if err != nil || !sameFrozenBrain(bootstrap.AttestedBrain, want) {
		return cognitionpolicy.BrainBootstrap{}, fmt.Errorf("runtime Brain bootstrap evidence differs from the frozen Brain")
	}
	return bootstrap, nil
}

func (authority RuntimeBrainBootstrapEvidenceAuthority) Validate() error {
	if authority.Schema != RuntimeBrainBootstrapEvidenceAuthoritySchemaV1 ||
		authority.ID != "runtime_brain_bootstrap_evidence_"+authority.SHA256 ||
		!validDigest(authority.SHA256) || authority.Bytes < 1 ||
		authority.Bytes > maxRuntimeBrainBootstrapEvidenceBytes ||
		authority.Evidence.Validate() != nil ||
		authority.File != "runtime-brain-bootstrap-"+authority.SHA256+".json" ||
		filepath.Base(authority.File) != authority.File {
		return fmt.Errorf("runtime Brain bootstrap evidence authority is invalid")
	}
	return nil
}

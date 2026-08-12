package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/llm"
)

const (
	RuntimeBrainBootstrapEvidenceArtifactSchemaV1  = "omnidex.runtime-brain-bootstrap-evidence.v1"
	RuntimeBrainBootstrapEvidenceAuthoritySchemaV1 = "omnidex.runtime-brain-bootstrap-evidence-authority.v1"
	maxRuntimeBrainBootstrapEvidenceBytes          = 48 * 1024 * 1024
)

type RuntimeBrainBootstrapEvidenceAuthority struct {
	Schema   string                          `json:"schema"`
	ID       string                          `json:"id"`
	SHA256   string                          `json:"sha256"`
	Bytes    int                             `json:"bytes"`
	File     string                          `json:"file"`
	Evidence llm.ProviderIdentityEvidenceRef `json:"evidence"`
}

type runtimeBrainBootstrapEvidenceArtifact struct {
	Schema  string                          `json:"schema"`
	Brain   cognitionpolicy.AttestedBrain   `json:"brain"`
	Capture preparedProviderIdentityCapture `json:"capture"`
}

func prepareRuntimeBrainBootstrapEvidence(
	episodePath string,
	bootstrap cognitionpolicy.BrainBootstrap,
	frozen BrainFingerprint,
) (
	runtimeBrainBootstrapEvidenceArtifact,
	RuntimeBrainBootstrapEvidenceAuthority,
	error,
) {
	if err := bootstrap.Validate(); err != nil {
		return runtimeBrainBootstrapEvidenceArtifact{}, RuntimeBrainBootstrapEvidenceAuthority{}, err
	}
	observed := llm.ObservedProviderIdentity{
		Attestation: bootstrap.AttestedBrain.Attestation,
		Observation: bootstrap.AttestedBrain.BootstrapObservation,
		Evidence:    bootstrap.BootstrapEvidence,
	}
	capture, err := newPreparedProviderIdentityCapture(observed)
	if err != nil {
		return runtimeBrainBootstrapEvidenceArtifact{}, RuntimeBrainBootstrapEvidenceAuthority{}, err
	}
	artifact, err := finalizeRuntimeBrainBootstrapEvidenceArtifact(
		runtimeBrainBootstrapEvidenceArtifact{
			Schema: RuntimeBrainBootstrapEvidenceArtifactSchemaV1,
			Brain:  bootstrap.AttestedBrain, Capture: capture,
		},
		frozen,
	)
	if err != nil {
		return runtimeBrainBootstrapEvidenceArtifact{}, RuntimeBrainBootstrapEvidenceAuthority{}, err
	}
	raw, err := encodeRuntimeBrainBootstrapEvidenceArtifact(artifact)
	if err != nil {
		return runtimeBrainBootstrapEvidenceArtifact{}, RuntimeBrainBootstrapEvidenceAuthority{}, err
	}
	digest := digestExactBytes(raw)
	authority := RuntimeBrainBootstrapEvidenceAuthority{
		Schema: RuntimeBrainBootstrapEvidenceAuthoritySchemaV1,
		ID:     "runtime_brain_bootstrap_evidence_" + digest,
		SHA256: digest, Bytes: len(raw),
		File:     "runtime-brain-bootstrap-" + digest + ".json",
		Evidence: artifact.Capture.EvidenceManifest.Ref,
	}
	if err := authority.Validate(); err != nil {
		return runtimeBrainBootstrapEvidenceArtifact{}, RuntimeBrainBootstrapEvidenceAuthority{}, err
	}
	if episodePath == "" {
		return runtimeBrainBootstrapEvidenceArtifact{}, RuntimeBrainBootstrapEvidenceAuthority{},
			fmt.Errorf("runtime Brain bootstrap episode path is empty")
	}
	return artifact, authority, nil
}

func finalizeRuntimeBrainBootstrapEvidenceArtifact(
	artifact runtimeBrainBootstrapEvidenceArtifact,
	frozen BrainFingerprint,
) (runtimeBrainBootstrapEvidenceArtifact, error) {
	if _, err := artifact.verifyFor(frozen); err != nil {
		return runtimeBrainBootstrapEvidenceArtifact{}, err
	}
	return artifact, nil
}

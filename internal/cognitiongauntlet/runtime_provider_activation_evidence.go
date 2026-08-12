package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/llm"
)

const (
	RuntimeProviderActivationEvidenceArtifactSchemaV1  = "omnidex.runtime-provider-activation-evidence.v1"
	RuntimeProviderActivationEvidenceAuthoritySchemaV1 = "omnidex.runtime-provider-activation-evidence-authority.v1"
	maxRuntimeProviderActivationEvidenceBytes          = 48 * 1024 * 1024
)

type RuntimeProviderActivationEvidenceAuthority struct {
	Schema        string                          `json:"schema"`
	ID            string                          `json:"id"`
	SHA256        string                          `json:"sha256"`
	Bytes         int                             `json:"bytes"`
	File          string                          `json:"file"`
	ObservationID string                          `json:"observation_id"`
	Evidence      llm.ProviderIdentityEvidenceRef `json:"evidence"`
}

type runtimeProviderActivationEvidenceArtifact struct {
	Schema  string                                     `json:"schema"`
	Receipt cognitionpolicy.ProviderProcessObservation `json:"receipt"`
	Capture preparedProviderIdentityCapture            `json:"capture"`
}

func prepareRuntimeProviderActivationEvidence(
	episodePath string,
	activation cognitionpolicy.ProviderProcessActivation,
	frozen BrainFingerprint,
) (
	runtimeProviderActivationEvidenceArtifact,
	RuntimeProviderActivationEvidenceAuthority,
	error,
) {
	brain, err := frozen.attestedBrain()
	if err != nil || activation.ValidateFor(brain) != nil {
		return runtimeProviderActivationEvidenceArtifact{}, RuntimeProviderActivationEvidenceAuthority{},
			fmt.Errorf("runtime provider activation differs from the frozen Brain")
	}
	observed := llm.ObservedProviderIdentity{
		Attestation: brain.Attestation,
		Observation: activation.Receipt.Observation,
		Evidence:    activation.IdentityEvidence,
	}
	capture, err := newPreparedProviderIdentityCapture(observed)
	if err != nil {
		return runtimeProviderActivationEvidenceArtifact{}, RuntimeProviderActivationEvidenceAuthority{}, err
	}
	artifact := runtimeProviderActivationEvidenceArtifact{
		Schema:  RuntimeProviderActivationEvidenceArtifactSchemaV1,
		Receipt: activation.Receipt, Capture: capture,
	}
	if _, err := artifact.verifyFor(frozen); err != nil {
		return runtimeProviderActivationEvidenceArtifact{}, RuntimeProviderActivationEvidenceAuthority{}, err
	}
	raw, err := encodeRuntimeProviderActivationEvidenceArtifact(artifact)
	if err != nil {
		return runtimeProviderActivationEvidenceArtifact{}, RuntimeProviderActivationEvidenceAuthority{}, err
	}
	digest := digestExactBytes(raw)
	authority := RuntimeProviderActivationEvidenceAuthority{
		Schema: RuntimeProviderActivationEvidenceAuthoritySchemaV1,
		ID:     "runtime_provider_activation_evidence_" + digest,
		SHA256: digest, Bytes: len(raw),
		File:          "runtime-provider-activation-" + digest + ".json",
		ObservationID: activation.Receipt.ID,
		Evidence:      activation.IdentityEvidence.Ref,
	}
	if err := authority.Validate(); err != nil {
		return runtimeProviderActivationEvidenceArtifact{}, RuntimeProviderActivationEvidenceAuthority{}, err
	}
	if episodePath == "" {
		return runtimeProviderActivationEvidenceArtifact{}, RuntimeProviderActivationEvidenceAuthority{},
			fmt.Errorf("runtime provider activation episode path is empty")
	}
	return artifact, authority, nil
}

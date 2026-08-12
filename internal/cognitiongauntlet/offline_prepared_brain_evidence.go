package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/llm"
)

const (
	PreparedBrainEvidenceArtifactSchemaV1  = "omnidex.prepared-brain-evidence.v1"
	PreparedBrainEvidenceAuthoritySchemaV1 = "omnidex.prepared-brain-evidence-authority.v1"
	maxPreparedBrainEvidenceArtifactBytes  = 96 * 1024 * 1024
)

type PreparedBrainEvidenceAuthority struct {
	Schema            string                          `json:"schema"`
	ID                string                          `json:"id"`
	SHA256            string                          `json:"sha256"`
	Bytes             int                             `json:"bytes"`
	Path              string                          `json:"path"`
	DiscoveryEvidence llm.ProviderIdentityEvidenceRef `json:"discovery_evidence"`
	BootstrapEvidence llm.ProviderIdentityEvidenceRef `json:"bootstrap_evidence"`
}

type preparedProviderIdentityCapture struct {
	Attestation      llm.ProviderIdentityAttestation `json:"attestation"`
	Observation      llm.ProviderIdentityObservation `json:"observation"`
	EvidenceManifest llm.ProviderIdentityEvidence    `json:"evidence_manifest"`
	Requests         [][]byte                        `json:"requests"`
	Responses        [][]byte                        `json:"responses"`
}

type preparedBrainEvidenceArtifact struct {
	Schema    string                          `json:"schema"`
	Discovery preparedProviderIdentityCapture `json:"discovery"`
	Bootstrap preparedProviderIdentityCapture `json:"bootstrap"`
}

type verifiedPreparedBrainEvidence struct {
	discovery llm.ObservedProviderIdentity
	bootstrap llm.ObservedProviderIdentity
}

func newPreparedBrainEvidenceArtifact(
	discovery llm.ObservedProviderIdentity,
	bootstrap llm.ObservedProviderIdentity,
	brain BrainFingerprint,
) (preparedBrainEvidenceArtifact, error) {
	discoveryCapture, err := newPreparedProviderIdentityCapture(discovery)
	if err != nil {
		return preparedBrainEvidenceArtifact{}, fmt.Errorf("capture discovery evidence: %w", err)
	}
	bootstrapCapture, err := newPreparedProviderIdentityCapture(bootstrap)
	if err != nil {
		return preparedBrainEvidenceArtifact{}, fmt.Errorf("capture bootstrap evidence: %w", err)
	}
	return finalizePreparedBrainEvidenceArtifact(preparedBrainEvidenceArtifact{
		Schema:    PreparedBrainEvidenceArtifactSchemaV1,
		Discovery: discoveryCapture, Bootstrap: bootstrapCapture,
	}, brain)
}

func newPreparedProviderIdentityCapture(
	observed llm.ObservedProviderIdentity,
) (preparedProviderIdentityCapture, error) {
	owned, err := llm.OwnBoundedProviderIdentityEvidence(observed.Evidence)
	if err != nil {
		return preparedProviderIdentityCapture{}, err
	}
	requests := make([][]byte, len(owned.Operations))
	responses := make([][]byte, len(owned.Operations))
	manifest := owned.Clone()
	for index := range manifest.Operations {
		requests[index] = append([]byte(nil), manifest.Operations[index].Request...)
		responses[index] = append([]byte(nil), manifest.Operations[index].ResponseCapture...)
		manifest.Operations[index].Request = nil
		manifest.Operations[index].ResponseCapture = nil
	}
	return preparedProviderIdentityCapture{
		Attestation: observed.Attestation, Observation: observed.Observation,
		EvidenceManifest: manifest, Requests: requests, Responses: responses,
	}, nil
}

func (capture preparedProviderIdentityCapture) observed() (llm.ObservedProviderIdentity, error) {
	if len(capture.EvidenceManifest.Operations) != 5 || len(capture.Requests) != 5 ||
		len(capture.Responses) != 5 {
		return llm.ObservedProviderIdentity{}, fmt.Errorf("prepared provider evidence requires five exact operations")
	}
	evidence := capture.EvidenceManifest.Clone()
	for index := range evidence.Operations {
		if len(evidence.Operations[index].Request) != 0 ||
			len(evidence.Operations[index].ResponseCapture) != 0 {
			return llm.ObservedProviderIdentity{}, fmt.Errorf("prepared provider manifest contains loose raw bodies")
		}
		evidence.Operations[index].Request = append([]byte(nil), capture.Requests[index]...)
		evidence.Operations[index].ResponseCapture = append([]byte(nil), capture.Responses[index]...)
	}
	owned, err := llm.OwnBoundedProviderIdentityEvidence(evidence)
	if err != nil {
		return llm.ObservedProviderIdentity{}, err
	}
	return llm.ObservedProviderIdentity{
		Attestation: capture.Attestation, Observation: capture.Observation, Evidence: owned,
	}, nil
}

func finalizePreparedBrainEvidenceArtifact(
	artifact preparedBrainEvidenceArtifact,
	brain BrainFingerprint,
) (preparedBrainEvidenceArtifact, error) {
	if _, err := artifact.verifyFor(brain); err != nil {
		return preparedBrainEvidenceArtifact{}, err
	}
	return artifact, nil
}

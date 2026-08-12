package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/llm"
)

func (artifact preparedBrainEvidenceArtifact) verifyFor(
	brain BrainFingerprint,
) (verifiedPreparedBrainEvidence, error) {
	if artifact.Schema != PreparedBrainEvidenceArtifactSchemaV1 {
		return verifiedPreparedBrainEvidence{}, fmt.Errorf("prepared brain evidence artifact identity is invalid")
	}
	if err := brain.Validate(); err != nil {
		return verifiedPreparedBrainEvidence{}, err
	}
	discovery, err := artifact.Discovery.observed()
	if err != nil {
		return verifiedPreparedBrainEvidence{}, fmt.Errorf("reconstruct discovery evidence: %w", err)
	}
	bootstrap, err := artifact.Bootstrap.observed()
	if err != nil {
		return verifiedPreparedBrainEvidence{}, fmt.Errorf("reconstruct bootstrap evidence: %w", err)
	}
	selection := llm.ProviderIdentitySelection{
		Model: brain.Model, NativeContextLimit: brain.NativeContextLimit,
	}
	discoveryChallenge, err := llm.DeriveProviderIdentityDiscoveryChallenge(
		offlineProviderDiscoveryScopeV1, selection,
	)
	if err != nil {
		return verifiedPreparedBrainEvidence{}, err
	}
	expected, err := llm.DeriveExactProviderIdentityExpectation(discovery.Evidence, selection)
	if err != nil || discovery.ValidateFor(llm.ProviderIdentityObservationRequest{
		Expectation: expected, ChallengeSHA256: discoveryChallenge,
	}) != nil {
		return verifiedPreparedBrainEvidence{}, fmt.Errorf("prepared discovery evidence is not independently reproducible")
	}
	samplingSHA256, err := brain.Sampling.SHA256()
	if err != nil {
		return verifiedPreparedBrainEvidence{}, err
	}
	bootstrapChallenge, err := llm.DeriveProviderIdentityObservationChallenge(
		"cognition-brain-bootstrap:"+samplingSHA256, expected,
	)
	if err != nil || bootstrap.ValidateFor(llm.ProviderIdentityObservationRequest{
		Expectation: expected, ChallengeSHA256: bootstrapChallenge,
	}) != nil {
		return verifiedPreparedBrainEvidence{}, fmt.Errorf("prepared bootstrap evidence is not independently reproducible")
	}
	if discovery.Attestation != brain.ProviderAttestation ||
		bootstrap.Attestation != brain.ProviderAttestation ||
		bootstrap.Observation != brain.ProviderObservation {
		return verifiedPreparedBrainEvidence{}, fmt.Errorf("prepared raw evidence differs from the frozen Brain fingerprint")
	}
	attested, err := brain.attestedBrain()
	if err != nil {
		return verifiedPreparedBrainEvidence{}, err
	}
	if _, err := cognitionpolicy.NewBrainBootstrap(attested, bootstrap.Evidence); err != nil {
		return verifiedPreparedBrainEvidence{}, fmt.Errorf("prepared Brain bootstrap proof is invalid: %w", err)
	}
	return verifiedPreparedBrainEvidence{discovery: discovery, bootstrap: bootstrap}, nil
}

func (authority PreparedBrainEvidenceAuthority) validateShape() error {
	if authority.Schema != PreparedBrainEvidenceAuthoritySchemaV1 ||
		authority.ID != "prepared_brain_evidence_"+authority.SHA256 ||
		!validDigest(authority.SHA256) || authority.Bytes < 1 ||
		authority.Bytes > maxPreparedBrainEvidenceArtifactBytes ||
		authority.DiscoveryEvidence.Validate() != nil ||
		authority.BootstrapEvidence.Validate() != nil {
		return fmt.Errorf("prepared brain evidence authority is invalid")
	}
	return validateExactAbsolutePath(authority.Path, "prepared brain evidence artifact")
}

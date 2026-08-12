package cognitiongauntlet

import (
	"fmt"
	"os"

	"github.com/gryph/omnidex/internal/cognitionreplay"
)

// ExportAblationSemanticReplay builds an unqualified public projection from
// the exact cold ablation evidence. Oracle-contaminated evidence is private
// and cannot enter this public container before a verified private overlay is
// available.
func ExportAblationSemanticReplay(
	bundle PublicInferenceBundle,
	episode SealedEpisode,
	episodePath string,
	evidencePath string,
) (cognitionreplay.Artifact, error) {
	if err := validateAblationSemanticAuthority(bundle, episode); err != nil {
		return cognitionreplay.Artifact{}, err
	}
	class, private, err := ablationReplayClass(bundle.Authority.Variant)
	if err != nil {
		return cognitionreplay.Artifact{}, err
	}
	if private {
		return cognitionreplay.Artifact{}, fmt.Errorf(
			"oracle-contaminated ablation replay requires a verified private overlay",
		)
	}
	expected, err := NewAblationEvidenceExpectation(bundle.Authority, episode)
	if err != nil {
		return cognitionreplay.Artifact{}, err
	}
	if _, err := VerifyAblationEvidenceFor(evidencePath, episode, expected); err != nil {
		return cognitionreplay.Artifact{}, err
	}
	authority, err := ablationEvidenceAuthorityFromEpisode(episode)
	if err != nil {
		return cognitionreplay.Artifact{}, err
	}
	evidence, err := loadAblationEvidence(evidencePath, authority)
	if err != nil {
		return cognitionreplay.Artifact{}, err
	}
	bootstrapRaw, activationRaw, err := readAblationRuntimeArtifacts(episodePath, episode)
	if err != nil {
		return cognitionreplay.Artifact{}, err
	}
	input, err := buildAblationSemanticReplay(
		bundle, episode, evidence, bootstrapRaw, activationRaw, class,
	)
	if err != nil {
		return cognitionreplay.Artifact{}, err
	}
	return cognitionreplay.ExportAblationSemanticProjection(input)
}

func readAblationRuntimeArtifacts(
	episodePath string,
	episode SealedEpisode,
) ([]byte, []byte, error) {
	var bootstrap RuntimeBrainBootstrapEvidenceAuthority
	var activation RuntimeProviderActivationEvidenceAuthority
	bootstrapCount, activationCount := 0, 0
	for _, entry := range episode.Manifest.Trace {
		switch entry.Kind {
		case TraceProviderBootstrap:
			value, err := decodeRuntimeBrainBootstrapTrace(entry)
			if err != nil {
				return nil, nil, err
			}
			bootstrap, bootstrapCount = value, bootstrapCount+1
		case TraceProviderActivation:
			value, err := decodeRuntimeProviderActivationTrace(entry)
			if err != nil {
				return nil, nil, err
			}
			activation, activationCount = value, activationCount+1
		}
	}
	if bootstrapCount != 1 || activationCount != 1 {
		return nil, nil, fmt.Errorf("ablation replay lacks exact runtime identity authorities")
	}
	frozen := episode.Manifest.RatGeneration.Fixed.Brain
	if _, err := loadRuntimeBrainBootstrapEvidence(episodePath, bootstrap, frozen); err != nil {
		return nil, nil, err
	}
	if _, err := loadRuntimeProviderActivationEvidence(episodePath, activation, frozen); err != nil {
		return nil, nil, err
	}
	bootstrapRaw, err := os.ReadFile(runtimeBrainBootstrapEvidencePath(episodePath, bootstrap))
	if err != nil || len(bootstrapRaw) != bootstrap.Bytes || digestExactBytes(bootstrapRaw) != bootstrap.SHA256 {
		return nil, nil, fmt.Errorf("read exact ablation runtime Brain bootstrap: %v", err)
	}
	activationRaw, err := os.ReadFile(runtimeProviderActivationEvidencePath(episodePath, activation))
	if err != nil || len(activationRaw) != activation.Bytes || digestExactBytes(activationRaw) != activation.SHA256 {
		return nil, nil, fmt.Errorf("read exact ablation runtime provider activation: %v", err)
	}
	return bootstrapRaw, activationRaw, nil
}

func validateAblationSemanticAuthority(
	bundle PublicInferenceBundle,
	episode SealedEpisode,
) error {
	if err := bundle.Validate(); err != nil {
		return err
	}
	if err := episode.Validate(); err != nil {
		return err
	}
	if !executableAblation(bundle.Authority.Variant) ||
		episode.Manifest.Variant != bundle.Authority.Variant {
		return fmt.Errorf("ablation semantic replay variant is invalid")
	}
	publicSHA, err := bundle.Authority.SHA256()
	if err != nil || episode.Manifest.PublicRunAuthoritySHA256 != publicSHA ||
		episode.Manifest.Scenario != bundle.Authority.Scenario ||
		episode.Manifest.RatGeneration != bundle.Authority.RatGeneration ||
		episode.Manifest.StationBudget != bundle.Authority.Budget.Station {
		return fmt.Errorf("ablation semantic episode differs from its public bundle")
	}
	return nil
}

package cognitiongauntlet

import (
	"bytes"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionreplay"
)

func verifyAblationSemanticProjection(
	verified cognitionreplay.VerifiedBase,
) (ablationSemanticProjectionVerification, error) {
	manifest := verified.Manifest()
	authority := manifest.AblationProjectionAuthority
	if manifest.SemanticStatus != cognitionreplay.SemanticAblationProjection || authority == nil {
		return ablationSemanticProjectionVerification{}, fmt.Errorf(
			"replay is not an ablation semantic projection",
		)
	}
	var bundle PublicInferenceBundle
	if err := decodeSemanticAuthorityBlob(
		verified, authority.PublicBundle, &bundle, "ablation public bundle",
	); err != nil {
		return ablationSemanticProjectionVerification{}, err
	}
	var episode SealedEpisode
	if err := decodeSemanticAuthorityBlob(
		verified, authority.SealedEpisode, &episode, "ablation sealed episode",
	); err != nil {
		return ablationSemanticProjectionVerification{}, err
	}
	evidenceRaw, err := verified.ProjectionContent(authority.AblationEvidence)
	if err != nil {
		return ablationSemanticProjectionVerification{}, err
	}
	var evidence ablationEvidenceArtifact
	if err := decodeStrictJSON(evidenceRaw, &evidence, "embedded ablation evidence"); err != nil {
		return ablationSemanticProjectionVerification{}, err
	}
	canonicalEvidence, err := encodeAblationEvidenceArtifact(evidence)
	if err != nil || !bytes.Equal(evidenceRaw, canonicalEvidence) {
		return ablationSemanticProjectionVerification{}, fmt.Errorf(
			"embedded ablation evidence is not exact canonical JSON",
		)
	}
	bootstrapRaw, err := verified.ProjectionContent(authority.BrainBootstrap)
	if err != nil {
		return ablationSemanticProjectionVerification{}, err
	}
	activationRaw, err := verified.ProjectionContent(authority.ProviderActivation)
	if err != nil {
		return ablationSemanticProjectionVerification{}, err
	}
	class, err := replayClassFromProjection(authority.ClaimedClass)
	if err != nil || authority.PrivateOverlayRequired {
		return ablationSemanticProjectionVerification{}, fmt.Errorf(
			"ablation semantic replay requires another evidence class boundary",
		)
	}
	expected, err := buildAblationSemanticReplay(
		bundle, episode, evidence, bootstrapRaw, activationRaw, class,
	)
	if err != nil {
		return ablationSemanticProjectionVerification{}, err
	}
	artifact, err := cognitionreplay.ExportAblationSemanticProjection(expected)
	if err != nil || artifact.SHA256 != verified.SHA256() {
		return ablationSemanticProjectionVerification{}, fmt.Errorf(
			"ablation semantic replay differs from exact typed rederivation: %v", err,
		)
	}
	return ablationSemanticProjectionVerification{
		verified: verified, bundle: bundle, episode: episode,
		evidence: evidence, class: class,
	}, nil
}

func replayClassFromProjection(
	class cognitionreplay.AblationProjectionClass,
) (AblationReplayClass, error) {
	switch class {
	case cognitionreplay.AblationProjectionSerious:
		return AblationReplaySerious, nil
	case cognitionreplay.AblationProjectionBenchmarkOnly:
		return AblationReplayBenchmarkOnly, nil
	default:
		return "", fmt.Errorf("ablation replay class is not publicly verifiable")
	}
}

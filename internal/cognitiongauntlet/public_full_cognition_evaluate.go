package cognitiongauntlet

import (
	"context"
	"fmt"
)

// EvaluatePublicFullCognition is the private, post-inference boundary. Callers
// must not load fixture or paired authority until the inference process has
// exited and the public episode has been sealed.
func EvaluatePublicFullCognition(
	path string,
	fixture MicrogauntletCase,
	paired PairedRunAuthority,
	public PublicRunAuthority,
	episode SealedEpisode,
) (FullCognitionRunResult, error) {
	if err := ValidatePublicRunAuthorityProjection(paired, public); err != nil {
		return FullCognitionRunResult{}, err
	}
	publicSHA, err := public.SHA256()
	if err != nil {
		return FullCognitionRunResult{}, err
	}
	if episode.Manifest.PublicRunAuthoritySHA256 != publicSHA ||
		episode.Manifest.Scenario != public.Scenario || episode.Manifest.Variant != public.Variant {
		return FullCognitionRunResult{}, fmt.Errorf(
			"private evaluator received an episode from another public run authority",
		)
	}
	evidence, err := derivePrivateEvaluationEvidence(
		context.Background(), fixture, paired.SurfaceVersion, episode,
	)
	if err != nil {
		return FullCognitionRunResult{}, err
	}
	evaluation, causal, err := ScoreAndSealMicrogauntletEpisode(
		path, fixture, paired.SurfaceVersion, episode, evidence,
	)
	if err != nil {
		return FullCognitionRunResult{}, err
	}
	oracle, err := fixture.oracleManifest()
	if err != nil {
		return FullCognitionRunResult{}, err
	}
	variant, err := BindVariantResult(paired, VariantFullCognition, episode, evaluation)
	if err != nil {
		return FullCognitionRunResult{}, err
	}
	efficiency, err := evaluation.EfficiencyMetric()
	if err != nil {
		return FullCognitionRunResult{}, err
	}
	result := FullCognitionRunResult{
		Authority: paired, Variant: variant, Episode: episode, Oracle: oracle,
		Evaluation: evaluation, Efficiency: efficiency, CausalAcquisition: causal,
	}
	return result, result.Validate()
}

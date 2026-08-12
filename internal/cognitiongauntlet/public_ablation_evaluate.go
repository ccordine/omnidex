package cognitiongauntlet

import (
	"context"
	"fmt"
)

// EvaluatePublicAblation performs deterministic private evaluation. This
// function alone is development evidence: it cannot prove process, credential,
// database-role, executable, or revocation isolation.
func EvaluatePublicAblation(
	path string,
	fixture MicrogauntletCase,
	paired PairedRunAuthority,
	public PublicRunAuthority,
	episode SealedEpisode,
) (AblationRunResult, error) {
	if !executableAblation(public.Variant) {
		return AblationRunResult{}, fmt.Errorf("private evaluator received a non-ablation variant")
	}
	if err := ValidatePublicRunAuthorityProjection(paired, public); err != nil {
		return AblationRunResult{}, err
	}
	publicSHA, err := public.SHA256()
	if err != nil {
		return AblationRunResult{}, err
	}
	if episode.Manifest.PublicRunAuthoritySHA256 != publicSHA ||
		episode.Manifest.Scenario != public.Scenario || episode.Manifest.Variant != public.Variant {
		return AblationRunResult{}, fmt.Errorf("private evaluator received another public ablation episode")
	}
	evidenceAuthority, err := ablationEvidenceAuthorityFromEpisode(episode)
	if err != nil {
		return AblationRunResult{}, err
	}
	evidence, err := derivePrivateEvaluationEvidence(
		context.Background(), fixture, paired.SurfaceVersion, episode,
	)
	if err != nil {
		return AblationRunResult{}, err
	}
	evaluation, causal, err := ScoreAndSealMicrogauntletEpisode(
		path, fixture, paired.SurfaceVersion, episode, evidence,
	)
	if err != nil {
		return AblationRunResult{}, err
	}
	oracle, err := fixture.oracleManifest()
	if err != nil {
		return AblationRunResult{}, err
	}
	variant, err := BindVariantResult(paired, public.Variant, episode, evaluation)
	if err != nil {
		return AblationRunResult{}, err
	}
	efficiency, err := evaluation.EfficiencyMetric()
	if err != nil {
		return AblationRunResult{}, err
	}
	class, eligible := AblationDevelopmentEvidence, false
	if public.Variant == VariantOracleEvidence {
		class, eligible = AblationOracleContaminated, false
	} else if public.Variant == VariantRawShell {
		class, eligible = AblationBenchmarkOnly, false
	}
	result := AblationRunResult{
		EvidenceClass: class, PromotionEligible: eligible,
		Authority: paired, Variant: variant, Episode: episode, Evidence: evidenceAuthority, Oracle: oracle,
		Evaluation: evaluation, Efficiency: efficiency, CausalAcquisition: causal,
	}
	return result, result.Validate()
}

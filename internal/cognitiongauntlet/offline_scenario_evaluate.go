package cognitiongauntlet

import (
	"context"
	"fmt"
)

func evaluatePublicOfflineScenario(
	path string,
	generated generatedOfflineScenario,
	paired PairedRunAuthority,
	public PublicRunAuthority,
	episode SealedEpisode,
) (Evaluation, CausalAcquisitionReport, error) {
	if err := ValidatePublicRunAuthorityProjection(paired, public); err != nil {
		return Evaluation{}, CausalAcquisitionReport{}, err
	}
	publicSHA, err := public.SHA256()
	if err != nil {
		return Evaluation{}, CausalAcquisitionReport{}, err
	}
	if episode.Manifest.PublicRunAuthoritySHA256 != publicSHA ||
		episode.Manifest.Scenario != public.Scenario || episode.Manifest.Variant != public.Variant {
		return Evaluation{}, CausalAcquisitionReport{}, fmt.Errorf(
			"private evaluator received another offline scenario episode",
		)
	}
	evidence, err := derivePrivateScenarioEvaluationEvidence(
		context.Background(), generated.scenario, paired.SurfaceVersion, episode,
	)
	if err != nil {
		return Evaluation{}, CausalAcquisitionReport{}, err
	}
	return scoreAndSealOfflineScenarioEpisode(
		path, generated, paired.SurfaceVersion, episode, evidence,
	)
}

func evaluateGeneratedPublicFullCognition(
	path string,
	generated generatedOfflineScenario,
	paired PairedRunAuthority,
	public PublicRunAuthority,
	episode SealedEpisode,
) (FullCognitionRunResult, error) {
	evaluation, causal, err := evaluatePublicOfflineScenario(
		path, generated, paired, public, episode,
	)
	if err != nil {
		return FullCognitionRunResult{}, err
	}
	oracle, err := generated.oracleManifest()
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

func evaluateGeneratedPublicAblation(
	path string,
	generated generatedOfflineScenario,
	paired PairedRunAuthority,
	public PublicRunAuthority,
	episode SealedEpisode,
) (AblationRunResult, error) {
	evaluation, causal, err := evaluatePublicOfflineScenario(
		path, generated, paired, public, episode,
	)
	if err != nil {
		return AblationRunResult{}, err
	}
	oracle, err := generated.oracleManifest()
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
	class := AblationDevelopmentEvidence
	if public.Variant == VariantOracleEvidence {
		class = AblationOracleContaminated
	} else if public.Variant == VariantRawShell {
		class = AblationBenchmarkOnly
	}
	result := AblationRunResult{
		EvidenceClass: class, PromotionEligible: false,
		Authority: paired, Variant: variant, Episode: episode, Oracle: oracle,
		Evaluation: evaluation, Efficiency: efficiency, CausalAcquisition: causal,
	}
	return result, result.Validate()
}

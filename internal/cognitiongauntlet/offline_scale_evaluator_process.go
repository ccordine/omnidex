package cognitiongauntlet

import (
	"context"
	"fmt"

	buildversion "github.com/gryph/omnidex/internal/version"
)

func RunOfflineScaleEvaluatorProcess(ctx context.Context, configPath string) error {
	if ctx == nil {
		return fmt.Errorf("offline Scale evaluator context is nil")
	}
	var config scaleEvaluatorProcessConfig
	if err := loadStrictJSONFile(
		configPath, &config, "offline Scale evaluator process configuration",
	); err != nil {
		return err
	}
	if err := config.Validate(); err != nil {
		return err
	}
	if err := validateCurrentProcessIdentity(
		config.ExecutableSHA256, config.OmnidexCommit, config.SourceSHA256,
		buildversion.Commit, buildversion.SourceSHA256,
	); err != nil {
		return err
	}
	private, err := loadPrivateScaleFixture(
		config.PrivateOraclePath, config.PrivateOracleCredential,
	)
	if err != nil {
		return err
	}
	if err := validateScaleProcessRuntime(
		private.Registration, config.ExecutableSHA256, config.SourceSHA256,
	); err != nil {
		return err
	}
	generated, err := private.regenerate()
	if err != nil {
		return err
	}
	bundle, err := LoadPublicInferenceBundle(config.PublicBundlePath)
	if err != nil {
		return err
	}
	if bundle.Authority.RatGeneration.Runtime.SourceSHA256 != config.SourceSHA256 ||
		bundle.Authority.RatGeneration.Runtime.ExecutableSHA256 != config.ExecutableSHA256 {
		return fmt.Errorf("offline Scale evaluator bundle changed build authority")
	}
	episode, err := LoadSealedEpisode(config.EpisodePath)
	if err != nil {
		return err
	}
	result, err := evaluateGeneratedPublicFullCognition(
		config.EvaluationPath, generated, private.Authority, bundle.Authority, episode,
	)
	if err != nil {
		return err
	}
	evaluation, evaluationSHA, err := LoadEvaluationArtifact(config.EvaluationPath)
	if err != nil {
		return err
	}
	if evaluation.EpisodeSealSHA256 != result.Episode.SealSHA256 ||
		evaluation.OracleSHA256 != result.Oracle.OracleSHA256 {
		return fmt.Errorf("offline Scale evaluator reloaded another evaluation")
	}
	evidence, err := buildOfflineScaleEvaluationEvidence(
		private, generated, result.Episode.SealSHA256, evaluationSHA,
	)
	if err != nil {
		return err
	}
	return SealOfflineScaleEvaluationEvidence(config.ScaleEvidencePath, evidence)
}

func buildOfflineScaleEvaluationEvidence(
	private privateScaleEvaluationFixture,
	generated generatedOfflineScenario,
	episodeSealSHA string,
	evaluationSHA string,
) (OfflineScaleEvaluationEvidence, error) {
	relevantBytes, err := labyrinthRelevantSurfaceBytes(*generated.initial)
	if err != nil {
		return OfflineScaleEvaluationEvidence{}, err
	}
	familySHA, err := digestJSON(private.Family)
	if err != nil {
		return OfflineScaleEvaluationEvidence{}, err
	}
	oracle := generated.initial.generated.PrivateOracle()
	evidence := OfflineScaleEvaluationEvidence{
		Schema: OfflineScaleEvaluationEvidenceSchemaV1, Case: private.Case,
		Family: private.Family, FamilySHA256: familySHA, Scenario: generated.scenario.Ref(),
		OracleSHA256: oracle.OracleSHA256, RelevantSurfaceBytes: relevantBytes,
		SolutionDepth:         private.Registration.BaseWorkload.Initial.Generator.Difficulty.SolutionDepth,
		RelevantEvidenceCount: len(oracle.RequiredEvidence),
		SemanticDecisionCount: len(oracle.Witness), EpisodeSealSHA256: episodeSealSHA,
		EvaluationArtifactSHA256: evaluationSHA,
	}
	return evidence, evidence.Validate()
}

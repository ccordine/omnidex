package cognitiongauntlet

import (
	"context"
	"fmt"
	"os"
	"reflect"

	buildversion "github.com/gryph/omnidex/internal/version"
)

func RunOfflineEvaluatorProcess(ctx context.Context, configPath string) error {
	if ctx == nil {
		return fmt.Errorf("offline evaluator process context is nil")
	}
	var config evaluatorProcessConfig
	if err := loadStrictJSONFile(configPath, &config, "offline evaluator process configuration"); err != nil {
		return err
	}
	if config.Schema != evaluatorProcessConfigSchemaV1 || config.PrivateOraclePath == "" ||
		!validDigest(config.ExecutableSHA256) || !validDigest(config.SourceSHA256) ||
		!validCommitIdentity(config.OmnidexCommit) {
		return fmt.Errorf("offline evaluator process configuration is invalid")
	}
	if err := requireExact(config.PrivateOracleCredential, "private oracle credential", 512); err != nil {
		return err
	}
	if err := validateCurrentProcessIdentity(
		config.ExecutableSHA256, config.OmnidexCommit, config.SourceSHA256,
		buildversion.Commit, buildversion.SourceSHA256,
	); err != nil {
		return err
	}
	if _, err := os.Stat(config.EvaluationPath); !os.IsNotExist(err) {
		return fmt.Errorf("offline evaluator output already exists or is inaccessible")
	}
	private, err := loadPrivateEvaluationFixture(
		config.PrivateOraclePath, config.PrivateOracleCredential,
	)
	if err != nil {
		return err
	}
	fixture, err := GenerateMicrogauntlet(private.Spec)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(fixture.generated.PrivateOracle(), private.Oracle) {
		return fmt.Errorf("private evaluator regeneration changed the sealed oracle")
	}
	paired, err := fixture.PairedAuthority(
		private.Surface, private.Authority.RatGeneration, private.Authority.Repetition,
		private.Authority.Runtime,
	)
	if err != nil || !reflect.DeepEqual(paired, private.Authority) {
		return fmt.Errorf("private evaluator regeneration changed the paired run authority")
	}
	bundle, err := LoadPublicInferenceBundle(config.PublicBundlePath)
	if err != nil {
		return err
	}
	if bundle.Authority.RatGeneration.Runtime.SourceSHA256 != config.SourceSHA256 ||
		bundle.Authority.RatGeneration.Runtime.ExecutableSHA256 != config.ExecutableSHA256 {
		return fmt.Errorf("offline evaluator bundle changed the attested build authority")
	}
	episode, err := LoadSealedEpisode(config.EpisodePath)
	if err != nil {
		return err
	}
	if bundle.Authority.Variant == VariantFullCognition {
		result, evaluateErr := EvaluatePublicFullCognition(
			config.EvaluationPath, fixture, paired, bundle.Authority, episode,
		)
		if evaluateErr != nil {
			return evaluateErr
		}
		return result.Validate()
	}
	result, err := EvaluatePublicAblation(
		config.EvaluationPath, fixture, paired, bundle.Authority, episode,
	)
	if err != nil {
		return err
	}
	return result.Validate()
}

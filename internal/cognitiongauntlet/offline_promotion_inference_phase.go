package cognitiongauntlet

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gryph/omnidex/internal/queue"
	buildversion "github.com/gryph/omnidex/internal/version"
)

type offlinePromotionInference struct {
	config                  OfflinePromotionConfig
	executable              string
	executableSHA256        string
	paths                   OfflinePromotionPaths
	bundle                  PublicInferenceBundle
	episode                 SealedEpisode
	privateOracleCredential string
	databaseSchema          string
	generatorPID            int
	generatorExitedAt       time.Time
	host                    OfflineHostReceipt
	inferencePID            int
	inferenceStartedAt      time.Time
	inferenceExitedAt       time.Time
}

func runOfflinePromotionInference(
	ctx context.Context,
	config OfflinePromotionConfig,
	executable string,
) (offlinePromotionInference, error) {
	if ctx == nil {
		return offlinePromotionInference{}, fmt.Errorf("offline cognition promotion context is nil")
	}
	if err := config.Validate(); err != nil {
		return offlinePromotionInference{}, err
	}
	if executable == "" || filepath.Clean(executable) != executable {
		return offlinePromotionInference{}, fmt.Errorf("offline cognition promotion executable is inexact")
	}
	executableSHA256, err := validateOfflinePromotionIdentity(
		config, executable, buildversion.Commit, buildversion.SourceSHA256,
		buildversion.MigrationsSHA256, buildversion.Version,
	)
	if err != nil {
		return offlinePromotionInference{}, err
	}
	migrations, err := loadReleaseMigrationBundle(executable, buildversion.MigrationsSHA256)
	if err != nil {
		return offlinePromotionInference{}, err
	}
	paths := config.Paths()
	temporary, err := os.MkdirTemp("", "omnidex-cognition-process-")
	if err != nil {
		return offlinePromotionInference{}, err
	}
	defer os.RemoveAll(temporary)
	hostScenarioPath := filepath.Join(temporary, "private-host-scenario.json")
	credential, err := randomProcessIdentity("oracle-credential-")
	if err != nil {
		return offlinePromotionInference{}, err
	}
	generatorPath := filepath.Join(temporary, "generator.json")
	generator := newGeneratorProcessConfig(
		config, paths, hostScenarioPath, credential, executableSHA256,
	)
	if err := writePrivateProcessFile(
		generatorPath, generator, "offline generator process configuration",
	); err != nil {
		return offlinePromotionInference{}, err
	}
	generatorPID, err := runOfflineChild(ctx, executable, "generate", generatorPath, executableSHA256)
	if err != nil {
		return offlinePromotionInference{}, err
	}
	generatorExitedAt := time.Now().UTC()
	bundle, err := LoadPublicInferenceBundle(paths.PublicBundle)
	if err != nil {
		return offlinePromotionInference{}, err
	}
	if err := os.Remove(generatorPath); err != nil {
		return offlinePromotionInference{}, fmt.Errorf("remove consumed generator authority: %w", err)
	}
	execution, err := runPreparedOfflineInference(
		ctx, config.executionAuthority(), executable, executableSHA256, migrations,
		paths, bundle, hostScenarioPath, credential, generatorPID,
		generatorExitedAt, temporary,
	)
	if err != nil {
		return offlinePromotionInference{}, err
	}
	return offlinePromotionInference{
		config: config, executable: executable, executableSHA256: executableSHA256,
		paths: paths, bundle: bundle, episode: execution.episode, privateOracleCredential: credential,
		databaseSchema: execution.databaseSchema, generatorPID: generatorPID,
		generatorExitedAt: generatorExitedAt, host: execution.host,
		inferencePID: execution.inferencePID, inferenceStartedAt: execution.inferenceStartedAt,
		inferenceExitedAt: execution.inferenceExitedAt,
	}, nil
}

func validatePublicInferenceEpisode(
	bundle PublicInferenceBundle,
	episode SealedEpisode,
) error {
	if bundle.Authority.Variant == VariantFullCognition {
		return (PublicFullCognitionRunResult{Authority: bundle.Authority, Episode: episode}).Validate()
	}
	evidence, err := ablationEvidenceAuthorityFromEpisode(episode)
	if err != nil {
		return err
	}
	return (PublicAblationRunResult{
		Authority: bundle.Authority, Episode: episode, Evidence: evidence,
	}).Validate()
}

func requireInferencePrivateOutputsAbsent(paths OfflinePromotionPaths) error {
	info, err := os.Stat(paths.PrivateOracle)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return fmt.Errorf("credentialed private oracle artifact is unavailable after inference")
	}
	if err := ensureAbsent(paths.Evaluation, "private cognition evaluation output"); err != nil {
		return err
	}
	return ensureAbsent(paths.Receipt, "private cognition promotion receipt")
}

func completeOfflineInferenceStep(ctx context.Context, database *offlinePromotionDatabase) error {
	operationID, err := queue.NewLifecycleOperationID(
		"offline-cognition", fmt.Sprint(database.attempt.JobID), "sealed",
	)
	if err != nil {
		return err
	}
	return database.repository.CompleteStep(ctx, queue.CompleteStepCommand{
		OperationID: operationID, Authority: database.attempt, StepID: database.attempt.StepID,
		Output: "Offline cognition inference exited after sealing its public episode.",
	})
}

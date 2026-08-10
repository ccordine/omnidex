package cognitiongauntlet

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gryph/omnidex/internal/queue"
	buildversion "github.com/gryph/omnidex/internal/version"
)

func RunOfflinePromotion(
	ctx context.Context,
	config OfflinePromotionConfig,
	executable string,
) (OfflinePromotionReceipt, error) {
	if ctx == nil {
		return OfflinePromotionReceipt{}, fmt.Errorf("offline cognition promotion context is nil")
	}
	if err := config.Validate(); err != nil {
		return OfflinePromotionReceipt{}, err
	}
	if executable == "" || filepath.Clean(executable) != executable {
		return OfflinePromotionReceipt{}, fmt.Errorf("offline cognition promotion executable is inexact")
	}
	executableSHA256, err := validateOfflinePromotionIdentity(
		config, executable, buildversion.Commit, buildversion.SourceSHA256,
		buildversion.MigrationsSHA256, buildversion.Version,
	)
	if err != nil {
		return OfflinePromotionReceipt{}, err
	}
	migrationsDirectory, err := releaseMigrationBundle(executable, buildversion.MigrationsSHA256)
	if err != nil {
		return OfflinePromotionReceipt{}, err
	}
	paths := config.Paths()
	temporary, err := os.MkdirTemp("", "omnidex-cognition-process-")
	if err != nil {
		return OfflinePromotionReceipt{}, err
	}
	defer os.RemoveAll(temporary)
	hostScenarioPath := filepath.Join(temporary, "private-host-scenario.json")
	generatorPath := filepath.Join(temporary, "generator.json")
	privateOracleCredential, err := randomProcessIdentity("oracle-credential-")
	if err != nil {
		return OfflinePromotionReceipt{}, err
	}
	generator := newGeneratorProcessConfig(
		config, paths, hostScenarioPath, privateOracleCredential, executableSHA256,
	)
	if err := writePrivateProcessFile(generatorPath, generator, "offline generator process configuration"); err != nil {
		return OfflinePromotionReceipt{}, err
	}
	generatorPID, err := runOfflineChild(ctx, executable, "generate", generatorPath, executableSHA256)
	if err != nil {
		return OfflinePromotionReceipt{}, err
	}
	generatorExitedAt := time.Now().UTC()
	bundle, err := LoadPublicInferenceBundle(paths.PublicBundle)
	if err != nil {
		return OfflinePromotionReceipt{}, err
	}
	if err := os.Remove(generatorPath); err != nil {
		return OfflinePromotionReceipt{}, fmt.Errorf("remove consumed generator authority: %w", err)
	}
	if err := verifyMigrationBundle(migrationsDirectory, buildversion.MigrationsSHA256); err != nil {
		return OfflinePromotionReceipt{}, err
	}
	database, err := prepareOfflinePromotionDatabase(ctx, config, migrationsDirectory)
	if err != nil {
		return OfflinePromotionReceipt{}, err
	}
	defer database.close()
	defer database.revokeInference(context.Background())
	defer database.revokeHost(context.Background())
	host, err := startOfflinePromotionHost(
		ctx, config, database, bundle, hostScenarioPath,
		executable, executableSHA256, temporary,
	)
	if err != nil {
		return OfflinePromotionReceipt{}, err
	}
	defer host.close(context.Background())
	inferencePath := filepath.Join(temporary, "inference.json")
	inference := newInferenceProcessConfig(
		config, database, host, paths, executableSHA256, privateOracleCredential,
	)
	if err := writePrivateProcessFile(inferencePath, inference, "offline inference process configuration"); err != nil {
		return OfflinePromotionReceipt{}, err
	}
	if err := database.enableInference(ctx); err != nil {
		return OfflinePromotionReceipt{}, err
	}
	inferencePID, err := runOfflineChild(
		ctx, executable, "infer", inferencePath, executableSHA256,
	)
	inferenceExitedAt := time.Now().UTC()
	revokeErr := database.revokeInference(context.Background())
	if err != nil || revokeErr != nil {
		return OfflinePromotionReceipt{}, errors.Join(err, revokeErr)
	}
	if err := host.close(ctx); err != nil {
		return OfflinePromotionReceipt{}, err
	}
	hostReceipt, err := host.receipt()
	if err != nil {
		return OfflinePromotionReceipt{}, err
	}
	if err := requireInferencePrivateOutputsAbsent(paths); err != nil {
		return OfflinePromotionReceipt{}, err
	}
	episode, err := LoadSealedEpisode(paths.Episode)
	if err != nil {
		return OfflinePromotionReceipt{}, err
	}
	if bundle.Authority.Variant == VariantFullCognition {
		if err := (PublicFullCognitionRunResult{Authority: bundle.Authority, Episode: episode}).Validate(); err != nil {
			return OfflinePromotionReceipt{}, err
		}
	} else if err := (PublicAblationRunResult{Authority: bundle.Authority, Episode: episode}).Validate(); err != nil {
		return OfflinePromotionReceipt{}, err
	}
	if err := completeOfflineInferenceStep(ctx, database); err != nil {
		return OfflinePromotionReceipt{}, err
	}
	evaluatorPath := filepath.Join(temporary, "evaluator.json")
	evaluator := newEvaluatorProcessConfig(config, paths, privateOracleCredential, executableSHA256)
	if err := writePrivateProcessFile(evaluatorPath, evaluator, "offline evaluator process configuration"); err != nil {
		return OfflinePromotionReceipt{}, err
	}
	evaluatorStartedAt := time.Now().UTC()
	evaluatorPID, err := runOfflineChild(
		ctx, executable, "evaluate", evaluatorPath, executableSHA256,
	)
	if err != nil {
		return OfflinePromotionReceipt{}, err
	}
	evaluation, evaluationArtifactSHA256, err := LoadEvaluationArtifact(paths.Evaluation)
	if err != nil {
		return OfflinePromotionReceipt{}, err
	}
	publicSHA, err := bundle.Authority.SHA256()
	if err != nil {
		return OfflinePromotionReceipt{}, err
	}
	receipt := OfflinePromotionReceipt{
		Schema:                   OfflinePromotionReceiptSchemaV1,
		PublicRunAuthoritySHA256: publicSHA, EpisodeSealSHA256: episode.SealSHA256,
		EvaluationOracleSHA256:   evaluation.OracleSHA256,
		EvaluationArtifactSHA256: evaluationArtifactSHA256, DatabaseSchema: database.schema,
		ExecutableSHA256: executableSHA256,
		SourceSHA256:     config.RatGeneration.Runtime.SourceSHA256,
		MigrationsSHA256: config.RatGeneration.Runtime.MigrationsSHA256,
		RuntimeVersion:   config.RatGeneration.Runtime.Version,
		OmnidexCommit:    config.OmnidexCommit,
		GeneratorPID:     generatorPID, GeneratorExitedAt: generatorExitedAt,
		Host:         hostReceipt,
		InferencePID: inferencePID, InferenceExitedAt: inferenceExitedAt,
		EvaluatorPID: evaluatorPID, EvaluatorStartedAt: evaluatorStartedAt,
		CompletedAt: time.Now().UTC(),
	}
	if err := receipt.Validate(); err != nil {
		return OfflinePromotionReceipt{}, err
	}
	if _, err := receipt.VerifyEvaluationArtifact(paths.Evaluation); err != nil {
		return OfflinePromotionReceipt{}, err
	}
	if err := sealScenarioArtifact(paths.Receipt, receipt, "offline cognition promotion receipt"); err != nil {
		return OfflinePromotionReceipt{}, err
	}
	return receipt, nil
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

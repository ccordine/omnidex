package cognitiongauntlet

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gryph/omnidex/internal/queue"
	buildversion "github.com/gryph/omnidex/internal/version"
)

func RunOfflineTakeover(
	ctx context.Context,
	config OfflineTakeoverConfig,
	executable string,
) (OfflineTakeoverReceipt, error) {
	if ctx == nil {
		return OfflineTakeoverReceipt{}, fmt.Errorf("offline cognition takeover context is nil")
	}
	if err := config.Validate(); err != nil {
		return OfflineTakeoverReceipt{}, err
	}
	promotion := config.Promotion
	executableSHA, err := validateOfflinePromotionIdentity(
		promotion, executable, buildversion.Commit, buildversion.SourceSHA256,
		buildversion.MigrationsSHA256, buildversion.Version,
	)
	if err != nil {
		return OfflineTakeoverReceipt{}, err
	}
	migrationsDirectory, err := releaseMigrationBundle(executable, buildversion.MigrationsSHA256)
	if err != nil {
		return OfflineTakeoverReceipt{}, err
	}
	paths := promotion.Paths()
	temporary, err := os.MkdirTemp("", "omnidex-cognition-takeover-")
	if err != nil {
		return OfflineTakeoverReceipt{}, err
	}
	defer os.RemoveAll(temporary)
	hostScenarioPath := takeoverProcessPath(temporary, "private-host-scenario")
	generatorPath := takeoverProcessPath(temporary, "generator-process")
	privateOracleCredential, err := randomProcessIdentity("oracle-credential-")
	if err != nil {
		return OfflineTakeoverReceipt{}, err
	}
	generator := newGeneratorProcessConfig(
		promotion, paths, hostScenarioPath, privateOracleCredential, executableSHA,
	)
	if err := writePrivateProcessFile(generatorPath, generator, "takeover generator process configuration"); err != nil {
		return OfflineTakeoverReceipt{}, err
	}
	generatorPID, err := runOfflineChild(ctx, executable, "generate", generatorPath, executableSHA)
	if err != nil {
		return OfflineTakeoverReceipt{}, err
	}
	generatorExitedAt := time.Now().UTC()
	bundle, err := LoadPublicInferenceBundle(paths.PublicBundle)
	if err != nil {
		return OfflineTakeoverReceipt{}, err
	}
	if err := os.Remove(generatorPath); err != nil {
		return OfflineTakeoverReceipt{}, fmt.Errorf("remove consumed generator authority: %w", err)
	}
	if err := verifyMigrationBundle(migrationsDirectory, buildversion.MigrationsSHA256); err != nil {
		return OfflineTakeoverReceipt{}, err
	}
	database, err := prepareOfflinePromotionDatabase(ctx, promotion, migrationsDirectory)
	if err != nil {
		return OfflineTakeoverReceipt{}, err
	}
	defer database.close()
	defer database.revokeInference(context.Background())
	defer database.revokeHost(context.Background())
	host, err := startOfflinePromotionHost(
		ctx, promotion, database, bundle, hostScenarioPath,
		executable, executableSHA, temporary,
	)
	if err != nil {
		return OfflineTakeoverReceipt{}, err
	}
	defer host.close(context.Background())
	before, originalPID, killedAt, err := runKilledInferencePrefix(
		ctx, promotion, database, host, paths, executable, executableSHA,
		config.AfterSuccessfulActions, temporary, bundle,
	)
	if err != nil {
		return OfflineTakeoverReceipt{}, err
	}
	reclaimCtx, cancelReclaim := context.WithTimeout(ctx, queue.StepAttemptLeaseDuration+30*time.Second)
	replacement, err := waitForReplacementClaim(reclaimCtx, database, database.attempt)
	cancelReclaim()
	if err != nil {
		return OfflineTakeoverReceipt{}, err
	}
	original := database.attempt
	database.attempt = replacement
	after, replacementPID, replacementExitedAt, err := runReplacementInference(
		ctx, promotion, database, host, paths, executable, executableSHA,
		config.AfterSuccessfulActions, temporary, bundle, before,
	)
	if err != nil {
		return OfflineTakeoverReceipt{}, err
	}
	continuity, err := NewTakeoverContinuityProof(before.PreCall, after.PreCall)
	if err != nil {
		return OfflineTakeoverReceipt{}, err
	}
	if err := database.revokeInference(context.Background()); err != nil {
		return OfflineTakeoverReceipt{}, err
	}
	if err := host.close(ctx); err != nil {
		return OfflineTakeoverReceipt{}, err
	}
	hostReceipt, err := host.receipt()
	if err != nil {
		return OfflineTakeoverReceipt{}, err
	}
	if err := requireInferencePrivateOutputsAbsent(paths); err != nil {
		return OfflineTakeoverReceipt{}, err
	}
	evaluation, evaluationArtifactSHA256, evaluatorPID, evaluatorStartedAt, episode, err := evaluateOfflineTakeover(
		ctx, promotion, database, bundle, paths, privateOracleCredential,
		executable, executableSHA, temporary,
	)
	if err != nil {
		return OfflineTakeoverReceipt{}, err
	}
	publicSHA, err := bundle.Authority.SHA256()
	if err != nil {
		return OfflineTakeoverReceipt{}, err
	}
	receipt := OfflineTakeoverReceipt{
		Schema:                   OfflineTakeoverReceiptSchemaV1,
		PublicRunAuthoritySHA256: publicSHA, EpisodeSealSHA256: episode.SealSHA256,
		EvaluationOracleSHA256:   evaluation.OracleSHA256,
		EvaluationArtifactSHA256: evaluationArtifactSHA256, ExecutableSHA256: executableSHA,
		SourceSHA256:     promotion.RatGeneration.Runtime.SourceSHA256,
		MigrationsSHA256: promotion.RatGeneration.Runtime.MigrationsSHA256,
		RuntimeVersion:   promotion.RatGeneration.Runtime.Version, OmnidexCommit: promotion.OmnidexCommit,
		GeneratorPID: generatorPID, GeneratorExitedAt: generatorExitedAt,
		Host:     hostReceipt,
		Original: original, Replacement: replacement, OriginalPID: originalPID,
		ReplacementPID: replacementPID, EvaluatorPID: evaluatorPID, OriginalKilledAt: killedAt,
		ReplacementExitedAt: replacementExitedAt, EvaluatorStartedAt: evaluatorStartedAt,
		CompletedAt: time.Now().UTC(), Continuity: continuity,
	}
	if err := receipt.Validate(); err != nil {
		return OfflineTakeoverReceipt{}, err
	}
	if _, err := receipt.VerifyEvaluationArtifact(paths.Evaluation); err != nil {
		return OfflineTakeoverReceipt{}, err
	}
	if err := sealScenarioArtifact(paths.Receipt, receipt, "offline cognition takeover receipt"); err != nil {
		return OfflineTakeoverReceipt{}, err
	}
	return receipt, nil
}

func requireKilledChild(child *offlineChildProcess) error {
	_, waitErr := child.wait()
	status, ok := child.command.ProcessState.Sys().(syscall.WaitStatus)
	if waitErr == nil || !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
		return fmt.Errorf("cognition inference did not terminate through the registered SIGKILL: %w", waitErr)
	}
	return nil
}

func ensureAbsent(path string, label string) error {
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s unexpectedly exists or is inaccessible", label)
	}
	return nil
}

func takeoverProcessPath(directory string, name string) string {
	return filepath.Join(directory, name+".json")
}

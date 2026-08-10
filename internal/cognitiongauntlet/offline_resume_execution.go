package cognitiongauntlet

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gryph/omnidex/internal/queue"
)

type offlineResumeExecution struct {
	config                  OfflinePromotionConfig
	executable              string
	executableSHA256        string
	paths                   OfflinePromotionPaths
	bundle                  PublicInferenceBundle
	privateOracleCredential string
	database                *offlinePromotionDatabase
	host                    *offlinePromotionHost
	generatorPID            int
	generatorExitedAt       time.Time
	inferenceStartedAt      time.Time
}

func startOfflineResumeExecution(
	ctx context.Context,
	config OfflinePromotionConfig,
	executable string,
	executableSHA string,
	migrations queue.MigrationBundle,
	temporary string,
) (*offlineResumeExecution, error) {
	paths := config.Paths()
	hostScenarioPath := filepath.Join(temporary, "private-host-scenario.json")
	credential, err := randomProcessIdentity("oracle-credential-")
	if err != nil {
		return nil, err
	}
	generatorPath := filepath.Join(temporary, "generator.json")
	generator := newGeneratorProcessConfig(
		config, paths, hostScenarioPath, credential, executableSHA,
	)
	if err := writePrivateProcessFile(
		generatorPath, generator, "Resume generator process configuration",
	); err != nil {
		return nil, err
	}
	generatorPID, err := runOfflineChild(ctx, executable, "generate", generatorPath, executableSHA)
	if err != nil {
		return nil, err
	}
	generatorExitedAt := time.Now().UTC()
	bundle, err := LoadPublicInferenceBundle(paths.PublicBundle)
	if err != nil {
		return nil, err
	}
	if err := os.Remove(generatorPath); err != nil {
		return nil, fmt.Errorf("remove consumed Resume generator authority: %w", err)
	}
	database, err := prepareOfflinePromotionDatabase(ctx, config, migrations)
	if err != nil {
		return nil, err
	}
	host, err := startOfflinePromotionHost(
		ctx, config, database, bundle, hostScenarioPath,
		executable, executableSHA, temporary,
	)
	if err != nil {
		database.close()
		return nil, err
	}
	return &offlineResumeExecution{
		config: config, executable: executable, executableSHA256: executableSHA,
		paths: paths, bundle: bundle, privateOracleCredential: credential,
		database: database, host: host, generatorPID: generatorPID,
		generatorExitedAt: generatorExitedAt, inferenceStartedAt: time.Now().UTC(),
	}, nil
}

func finishOfflineResumeExecution(
	ctx context.Context,
	setup *offlineResumeExecution,
	schedule OfflineResumeSchedule,
	interruptions []takeoverInterruption,
	finalPID int,
	finalExitedAt time.Time,
) (offlineResumeInference, error) {
	if setup == nil || setup.database == nil || setup.host == nil || finalPID <= 0 ||
		(len(interruptions) == 0 && schedule.Kind != ResumeUninterrupted) {
		return offlineResumeInference{}, fmt.Errorf("Resume inference completion authority is incomplete")
	}
	if err := setup.database.revokeInference(context.Background()); err != nil {
		return offlineResumeInference{}, err
	}
	if err := setup.host.close(ctx); err != nil {
		return offlineResumeInference{}, err
	}
	hostReceipt, err := setup.host.receipt()
	if err != nil {
		return offlineResumeInference{}, err
	}
	if err := requireInferencePrivateOutputsAbsent(setup.paths); err != nil {
		return offlineResumeInference{}, err
	}
	episode, err := LoadSealedEpisode(setup.paths.Episode)
	if err != nil {
		return offlineResumeInference{}, err
	}
	if err := validatePublicInferenceEpisode(setup.bundle, episode); err != nil {
		return offlineResumeInference{}, err
	}
	if err := completeOfflineInferenceStep(ctx, setup.database); err != nil {
		return offlineResumeInference{}, err
	}
	promotion := offlinePromotionInference{
		config: setup.config, executable: setup.executable,
		executableSHA256: setup.executableSHA256, paths: setup.paths,
		bundle: setup.bundle, episode: episode,
		privateOracleCredential: setup.privateOracleCredential,
		databaseSchema:          setup.database.schema, generatorPID: setup.generatorPID,
		generatorExitedAt: setup.generatorExitedAt, host: hostReceipt,
		inferencePID: finalPID, inferenceStartedAt: setup.inferenceStartedAt,
		inferenceExitedAt: finalExitedAt,
	}
	return offlineResumeInference{
		promotion: promotion, schedule: schedule,
		interruptions: append([]takeoverInterruption{}, interruptions...),
	}, nil
}

func (setup *offlineResumeExecution) close() {
	if setup == nil {
		return
	}
	if setup.host != nil {
		_ = setup.host.close(context.Background())
	}
	if setup.database != nil {
		_ = setup.database.revokeInference(context.Background())
		_ = setup.database.revokeHost(context.Background())
		setup.database.close()
	}
}

package cognitiongauntlet

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gryph/omnidex/internal/queue"
)

type offlineExecutionInference struct {
	authority          offlineExecutionAuthority
	executable         string
	executableSHA256   string
	paths              OfflinePromotionPaths
	bundle             PublicInferenceBundle
	episode            SealedEpisode
	privateCredential  string
	databaseSchema     string
	generatorPID       int
	generatorExitedAt  time.Time
	host               OfflineHostReceipt
	inferencePID       int
	inferenceStartedAt time.Time
	inferenceExitedAt  time.Time
}

func runPreparedOfflineInference(
	ctx context.Context,
	authority offlineExecutionAuthority,
	executable string,
	executableSHA256 string,
	migrations queue.MigrationBundle,
	paths OfflinePromotionPaths,
	bundle PublicInferenceBundle,
	hostScenarioPath string,
	privateCredential string,
	generatorPID int,
	generatorExitedAt time.Time,
	temporary string,
) (offlineExecutionInference, error) {
	if err := authority.Validate(); err != nil {
		return offlineExecutionInference{}, err
	}
	if bundle.Authority.RatGeneration != authority.RatGeneration ||
		bundle.Authority.Budget != authority.Budget ||
		bundle.Authority.Runtime != authority.RuntimeFingerprint ||
		bundle.Authority.Repetition != authority.Repetition ||
		bundle.Authority.Variant != authority.Variant {
		return offlineExecutionInference{}, fmt.Errorf("offline prepared bundle changed execution authority")
	}
	database, err := prepareOfflineExecutionDatabase(ctx, authority, migrations)
	if err != nil {
		return offlineExecutionInference{}, err
	}
	defer database.close()
	defer database.revokeInference(context.Background())
	defer database.revokeHost(context.Background())
	host, err := startOfflineExecutionHost(
		ctx, authority, database, bundle, hostScenarioPath, paths.PublicBundle,
		executable, executableSHA256, temporary,
	)
	if err != nil {
		return offlineExecutionInference{}, err
	}
	defer host.close(context.Background())
	inferencePath := filepath.Join(temporary, "inference.json")
	process := newInferenceProcessConfigForExecution(
		authority, database, host, paths, executableSHA256, privateCredential,
	)
	if err := writePrivateProcessFile(
		inferencePath, process, "offline inference process configuration",
	); err != nil {
		return offlineExecutionInference{}, err
	}
	if err := database.enableInference(ctx); err != nil {
		return offlineExecutionInference{}, err
	}
	inferenceStartedAt := time.Now().UTC()
	inferencePID, inferenceErr := runOfflineChild(
		ctx, executable, "infer", inferencePath, executableSHA256,
	)
	inferenceExitedAt := time.Now().UTC()
	revokeErr := database.revokeInference(context.Background())
	if inferenceErr != nil || revokeErr != nil {
		return offlineExecutionInference{}, errors.Join(inferenceErr, revokeErr)
	}
	if err := host.close(ctx); err != nil {
		return offlineExecutionInference{}, err
	}
	hostReceipt, err := host.receipt()
	if err != nil {
		return offlineExecutionInference{}, err
	}
	if err := requireInferencePrivateOutputsAbsent(paths); err != nil {
		return offlineExecutionInference{}, err
	}
	episode, err := LoadSealedEpisode(paths.Episode)
	if err != nil {
		return offlineExecutionInference{}, err
	}
	if err := validatePublicInferenceEpisode(bundle, episode); err != nil {
		return offlineExecutionInference{}, err
	}
	if err := completeOfflineInferenceStep(ctx, database); err != nil {
		return offlineExecutionInference{}, err
	}
	_ = os.Remove(inferencePath)
	return offlineExecutionInference{
		authority: authority, executable: executable, executableSHA256: executableSHA256,
		paths: paths, bundle: bundle, episode: episode, privateCredential: privateCredential,
		databaseSchema: database.schema, generatorPID: generatorPID,
		generatorExitedAt: generatorExitedAt, host: hostReceipt, inferencePID: inferencePID,
		inferenceStartedAt: inferenceStartedAt, inferenceExitedAt: inferenceExitedAt,
	}, nil
}

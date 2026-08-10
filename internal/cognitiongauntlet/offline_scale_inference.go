package cognitiongauntlet

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	buildversion "github.com/gryph/omnidex/internal/version"
)

type offlineScaleInference struct {
	current    OfflineScaleCase
	execution  offlineExecutionInference
	credential string
}

func runOfflineScaleInferences(
	ctx context.Context,
	config OfflineScaleConfig,
	registration OfflineScalePreregistration,
	executable string,
) ([]offlineScaleInference, time.Time, error) {
	authority := config.executionAuthority(registration.Cases[0])
	executableSHA, err := validateOfflineExecutionIdentity(
		authority, executable, buildversion.Commit, buildversion.SourceSHA256,
		buildversion.MigrationsSHA256, buildversion.Version,
	)
	if err != nil {
		return nil, time.Time{}, err
	}
	migrations, err := loadReleaseMigrationBundle(executable, buildversion.MigrationsSHA256)
	if err != nil {
		return nil, time.Time{}, err
	}
	temporary, err := os.MkdirTemp("", "omnidex-scale-process-")
	if err != nil {
		return nil, time.Time{}, err
	}
	defer os.RemoveAll(temporary)
	credential, err := randomProcessIdentity("scale-oracle-credential-")
	if err != nil {
		return nil, time.Time{}, err
	}
	for _, current := range registration.Cases {
		if err := config.createRunDirectories(current); err != nil {
			return nil, time.Time{}, err
		}
	}
	generator := newScaleGeneratorProcessConfig(
		config, registration, temporary, credential, executableSHA,
	)
	generatorPath := filepath.Join(temporary, "scale-generator.json")
	if err := writePrivateProcessFile(
		generatorPath, generator, "offline Scale generator process configuration",
	); err != nil {
		return nil, time.Time{}, err
	}
	generatorPID, err := runOfflineChild(
		ctx, executable, "generate-scale", generatorPath, executableSHA,
	)
	if err != nil {
		return nil, time.Time{}, err
	}
	generatorExitedAt := time.Now().UTC()
	if err := os.Remove(generatorPath); err != nil {
		return nil, time.Time{}, fmt.Errorf("remove consumed Scale generator authority: %w", err)
	}
	inferences := make([]offlineScaleInference, len(registration.Cases))
	lastExitedAt := time.Time{}
	for index, current := range registration.Cases {
		paths := config.runPaths(current)
		bundle, err := LoadPublicInferenceBundle(paths.PublicBundle)
		if err != nil {
			return nil, time.Time{}, err
		}
		execution, err := runPreparedOfflineInference(
			ctx, config.executionAuthority(current), executable, executableSHA, migrations,
			paths, bundle, generator.Outputs[index].HostScenarioPath, credential,
			generatorPID, generatorExitedAt, temporary,
		)
		if err != nil {
			return nil, time.Time{}, fmt.Errorf("execute Scale inference %d: %w", index+1, err)
		}
		if execution.inferenceStartedAt.Before(registration.RegisteredAt) {
			return nil, time.Time{}, fmt.Errorf("Scale inference started before preregistration")
		}
		if err := ensureAbsent(config.scaleEvidencePath(current), "private Scale evaluation evidence"); err != nil {
			return nil, time.Time{}, err
		}
		inferences[index] = offlineScaleInference{
			current: current, execution: execution, credential: credential,
		}
		if execution.inferenceExitedAt.After(lastExitedAt) {
			lastExitedAt = execution.inferenceExitedAt
		}
	}
	return inferences, lastExitedAt, nil
}

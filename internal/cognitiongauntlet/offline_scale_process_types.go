package cognitiongauntlet

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gryph/omnidex/internal/labyrinth"
)

const (
	scaleGeneratorProcessSchemaV1 = "omnidex.offline-scale-generator-process.v1"
	scaleEvaluatorProcessSchemaV1 = "omnidex.offline-scale-evaluator-process.v1"
	privateScaleFixtureSchemaV1   = "omnidex.private-scale-evaluation-fixture.v1"
)

type scaleProcessOutput struct {
	Case              OfflineScaleCase `json:"case"`
	PublicBundlePath  string           `json:"public_bundle_path"`
	HostScenarioPath  string           `json:"host_scenario_path"`
	PrivateOraclePath string           `json:"private_oracle_path"`
}

type scaleGeneratorProcessConfig struct {
	Schema                  string                      `json:"schema"`
	Registration            OfflineScalePreregistration `json:"registration"`
	Outputs                 []scaleProcessOutput        `json:"outputs"`
	PrivateOracleCredential string                      `json:"private_oracle_credential"`
	ExecutableSHA256        string                      `json:"executable_sha256"`
	SourceSHA256            string                      `json:"source_sha256"`
	OmnidexCommit           string                      `json:"omnidex_commit"`
}

type privateScaleEvaluationFixture struct {
	Schema       string                          `json:"schema"`
	Registration OfflineScalePreregistration     `json:"registration"`
	Case         OfflineScaleCase                `json:"case"`
	Family       labyrinth.ScaleFamilyDescriptor `json:"family"`
	Authority    PairedRunAuthority              `json:"authority"`
	Oracle       labyrinth.Oracle                `json:"oracle"`
}

type scaleEvaluatorProcessConfig struct {
	Schema                  string `json:"schema"`
	PrivateOraclePath       string `json:"private_oracle_path"`
	PrivateOracleCredential string `json:"private_oracle_credential"`
	PublicBundlePath        string `json:"public_bundle_path"`
	EpisodePath             string `json:"episode_path"`
	EvaluationPath          string `json:"evaluation_path"`
	ScaleEvidencePath       string `json:"scale_evidence_path"`
	ExecutableSHA256        string `json:"executable_sha256"`
	SourceSHA256            string `json:"source_sha256"`
	OmnidexCommit           string `json:"omnidex_commit"`
}

func (config scaleGeneratorProcessConfig) Validate() error {
	if config.Schema != scaleGeneratorProcessSchemaV1 || config.Registration.Validate() != nil ||
		len(config.Outputs) != config.Registration.RunCount ||
		!validDigest(config.ExecutableSHA256) || !validDigest(config.SourceSHA256) ||
		!validCommitIdentity(config.OmnidexCommit) {
		return fmt.Errorf("offline Scale generator process authority is invalid")
	}
	if err := requireExact(config.PrivateOracleCredential, "private Scale credential", 512); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(config.Outputs)*3)
	for index, output := range config.Outputs {
		if output.Case != config.Registration.Cases[index] {
			return fmt.Errorf("offline Scale generator output %d changed its coordinate", index+1)
		}
		for _, path := range []string{output.PublicBundlePath, output.HostScenarioPath, output.PrivateOraclePath} {
			if path == "" || filepath.Clean(path) != path {
				return fmt.Errorf("offline Scale generator output path is inexact")
			}
			if _, duplicate := seen[path]; duplicate {
				return fmt.Errorf("offline Scale generator output paths are duplicated")
			}
			seen[path] = struct{}{}
			if info, err := os.Stat(filepath.Dir(path)); err != nil || !info.IsDir() {
				return fmt.Errorf("offline Scale generator output directory is unavailable")
			}
			if _, err := os.Lstat(path); !os.IsNotExist(err) {
				return fmt.Errorf("offline Scale generator output already exists or is inaccessible")
			}
		}
	}
	return validateScaleProcessRuntime(
		config.Registration, config.ExecutableSHA256, config.SourceSHA256,
	)
}

func validateScaleProcessRuntime(
	registration OfflineScalePreregistration,
	executableSHA256 string,
	sourceSHA256 string,
) error {
	if registration.Fixed.RatGeneration.Runtime.ExecutableSHA256 != executableSHA256 ||
		registration.Fixed.RatGeneration.Runtime.SourceSHA256 != sourceSHA256 {
		return fmt.Errorf("offline Scale process changed its frozen runtime")
	}
	derived, err := currentRuntimeFingerprint(sourceSHA256)
	if err != nil || derived != registration.Fixed.RuntimeFingerprint {
		return fmt.Errorf("offline Scale process runtime is not code-derived")
	}
	return nil
}

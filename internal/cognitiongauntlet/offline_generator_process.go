package cognitiongauntlet

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gryph/omnidex/internal/labyrinth"
	buildversion "github.com/gryph/omnidex/internal/version"
)

const privateEvaluationFixtureSchemaV1 = "omnidex.private-evaluation-fixture.v1"

type privateEvaluationFixture struct {
	Schema    string             `json:"schema"`
	Spec      MicrogauntletSpec  `json:"spec"`
	Surface   Surface            `json:"surface"`
	Authority PairedRunAuthority `json:"authority"`
	Oracle    labyrinth.Oracle   `json:"oracle"`
}

func RunOfflineGeneratorProcess(ctx context.Context, configPath string) error {
	if ctx == nil {
		return fmt.Errorf("offline generator process context is nil")
	}
	var config generatorProcessConfig
	if err := loadStrictJSONFile(configPath, &config, "offline generator process configuration"); err != nil {
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
	fixture, err := GenerateMicrogauntlet(config.Spec)
	if err != nil {
		return err
	}
	paired, err := fixture.PairedAuthority(
		config.Surface, config.RatGeneration, config.Repetition, config.RuntimeFingerprint,
	)
	if err != nil {
		return err
	}
	bundle, err := NewVariantPublicInferenceBundle(fixture, paired, config.Variant)
	if err != nil {
		return err
	}
	hostRaw, err := fixture.SealedEnvironmentScenario().MarshalPrivateJSON()
	if err != nil {
		return err
	}
	private := privateEvaluationFixture{
		Schema: privateEvaluationFixtureSchemaV1, Spec: config.Spec, Surface: config.Surface,
		Authority: paired, Oracle: fixture.generated.PrivateOracle(),
	}
	if err := private.Validate(); err != nil {
		return err
	}
	if err := SealPublicInferenceBundle(config.PublicBundlePath, bundle); err != nil {
		return err
	}
	if err := writeExclusiveAtomic(config.HostScenarioPath, append(hostRaw, '\n')); err != nil {
		return fmt.Errorf("seal private host scenario: %w", err)
	}
	return sealCredentialedJSON(
		config.PrivateOraclePath, private, config.PrivateOracleCredential,
		"private cognition evaluation fixture",
	)
}

func (config generatorProcessConfig) Validate() error {
	if config.Schema != generatorProcessConfigSchemaV1 || config.Repetition <= 0 ||
		!validDigest(config.ExecutableSHA256) || !validDigest(config.SourceSHA256) ||
		!validCommitIdentity(config.OmnidexCommit) {
		return fmt.Errorf("offline generator process configuration is invalid")
	}
	if err := config.Spec.Validate(); err != nil {
		return err
	}
	if config.Variant != VariantFullCognition && !executableAblation(config.Variant) {
		return fmt.Errorf("offline generator variant %q is not executable", config.Variant)
	}
	if config.Variant == VariantRawShell && config.Surface != SurfaceFilesystem {
		return fmt.Errorf("offline generator raw shell requires the filesystem surface")
	}
	if _, err := config.Surface.Version(); err != nil {
		return err
	}
	if err := config.RatGeneration.Validate(); err != nil {
		return err
	}
	if err := config.Spec.Budget.ValidateFor(config.RatGeneration); err != nil {
		return err
	}
	if err := config.RuntimeFingerprint.Validate(); err != nil {
		return err
	}
	if err := requireExact(config.PrivateOracleCredential, "private oracle credential", 512); err != nil {
		return err
	}
	derivedRuntime, err := currentRuntimeFingerprint(config.SourceSHA256)
	if err != nil || config.RuntimeFingerprint != derivedRuntime ||
		config.RatGeneration.Runtime.SourceSHA256 != config.SourceSHA256 ||
		config.RatGeneration.Runtime.ExecutableSHA256 != config.ExecutableSHA256 {
		return fmt.Errorf("offline generator runtime authority is not code-derived")
	}
	paths := []string{config.PublicBundlePath, config.HostScenarioPath, config.PrivateOraclePath}
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if path == "" || filepath.Clean(path) != path {
			return fmt.Errorf("offline generator artifact path is inexact")
		}
		if _, duplicate := seen[path]; duplicate {
			return fmt.Errorf("offline generator artifact paths are not distinct")
		}
		seen[path] = struct{}{}
		if info, err := os.Stat(filepath.Dir(path)); err != nil || !info.IsDir() {
			return fmt.Errorf("offline generator artifact directory is unavailable")
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			return fmt.Errorf("offline generator artifact already exists or is inaccessible")
		}
	}
	return nil
}

func (fixture privateEvaluationFixture) Validate() error {
	if fixture.Schema != privateEvaluationFixtureSchemaV1 {
		return fmt.Errorf("private evaluation fixture schema is invalid")
	}
	if err := fixture.Spec.Validate(); err != nil {
		return err
	}
	surfaceVersion, err := fixture.Surface.Version()
	if err != nil || surfaceVersion != fixture.Authority.SurfaceVersion {
		return fmt.Errorf("private evaluation fixture surface is inconsistent")
	}
	if err := fixture.Authority.Validate(); err != nil {
		return err
	}
	if err := fixture.Oracle.Validate(); err != nil {
		return err
	}
	if fixture.Authority.CaseID != fixture.Spec.CaseID ||
		fixture.Authority.Seed != fixture.Spec.Generator.Seed ||
		fixture.Authority.OracleSHA256 != fixture.Oracle.OracleSHA256 ||
		fixture.Authority.Scenario.ID != fixture.Oracle.ScenarioID ||
		fixture.Authority.Scenario.SHA256 != fixture.Oracle.PublicSHA256 {
		return fmt.Errorf("private evaluation fixture authority is inconsistent")
	}
	return nil
}

func loadPrivateEvaluationFixture(path string, credential string) (privateEvaluationFixture, error) {
	var fixture privateEvaluationFixture
	if err := loadCredentialedJSON(
		path, &fixture, credential, "private cognition evaluation fixture",
	); err != nil {
		return privateEvaluationFixture{}, err
	}
	if err := fixture.Validate(); err != nil {
		return privateEvaluationFixture{}, err
	}
	return fixture, nil
}

func loadPrivateHostScenario(path string) (labyrinth.Scenario, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return labyrinth.Scenario{}, fmt.Errorf("read private host scenario: %w", err)
	}
	return labyrinth.ParsePrivateScenarioJSON(raw)
}

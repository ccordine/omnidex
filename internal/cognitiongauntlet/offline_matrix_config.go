package cognitiongauntlet

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

const (
	OfflineMatrixRequestSchemaV2 = "omnidex.offline-cognition-matrix-request.v2"
	OfflineMatrixConfigSchemaV2  = "omnidex.offline-cognition-matrix-config.v2"
)

type OfflineMatrixRequest struct {
	Schema                  string              `json:"schema"`
	Plan                    OfflineMatrixPlan   `json:"plan"`
	Budget                  RunBudget           `json:"budget"`
	DatabaseURL             string              `json:"database_url"`
	OllamaEndpoint          string              `json:"ollama_endpoint"`
	InferenceTimeoutSeconds int                 `json:"inference_timeout_seconds"`
	PublicOutputDirectory   string              `json:"public_output_directory"`
	PrivateOutputDirectory  string              `json:"private_output_directory"`
	Brain                   OfflineBrainRequest `json:"brain"`
}

type OfflineMatrixConfig struct {
	Schema                  string             `json:"schema"`
	Plan                    OfflineMatrixPlan  `json:"plan"`
	Budget                  RunBudget          `json:"budget"`
	DatabaseURL             string             `json:"database_url"`
	OllamaEndpoint          string             `json:"ollama_endpoint"`
	InferenceTimeoutSeconds int                `json:"inference_timeout_seconds"`
	PublicOutputDirectory   string             `json:"public_output_directory"`
	PrivateOutputDirectory  string             `json:"private_output_directory"`
	RatGeneration           RatGeneration      `json:"rat_generation"`
	RuntimeFingerprint      RuntimeFingerprint `json:"runtime_fingerprint"`
	PreregistrationSHA256   string             `json:"preregistration_sha256"`
	OmnidexCommit           string             `json:"omnidex_commit"`
	LedgerSchemaVersion     string             `json:"ledger_schema_version"`
	WorkingSetPolicyVersion string             `json:"working_set_policy_version"`
	ProjectionPolicyVersion string             `json:"projection_policy_version"`
}

type OfflineMatrixPaths struct {
	Preregistration string
	Receipt         string
}

func (request OfflineMatrixRequest) Validate() error {
	if request.Schema != OfflineMatrixRequestSchemaV2 || request.Plan.Validate() != nil ||
		request.Budget.Validate() != nil || request.Brain.NativeContextLimit <= 0 ||
		request.InferenceTimeoutSeconds <= 0 || request.InferenceTimeoutSeconds > 24*60*60 {
		return fmt.Errorf("offline cognition matrix request is invalid")
	}
	if request.Budget.Schema != RunBudgetSchemaStructuralV1 {
		return fmt.Errorf("offline matrix request requires the structural v1 budget authority")
	}
	if err := requireExact(request.Brain.Model, "offline matrix model", 512); err != nil {
		return err
	}
	if err := validateOfflineMatrixEndpoints(request.DatabaseURL, request.OllamaEndpoint); err != nil {
		return err
	}
	return validateOfflineOutputDirectories(
		request.PublicOutputDirectory, request.PrivateOutputDirectory,
	)
}

func (config OfflineMatrixConfig) Validate() error {
	if config.Schema != OfflineMatrixConfigSchemaV2 || config.Plan.Validate() != nil ||
		!validDigest(config.PreregistrationSHA256) || !validCommitIdentity(config.OmnidexCommit) ||
		config.InferenceTimeoutSeconds <= 0 || config.InferenceTimeoutSeconds > 24*60*60 {
		return fmt.Errorf("offline cognition matrix configuration is invalid")
	}
	if err := config.Budget.ValidateFor(config.RatGeneration); err != nil {
		return err
	}
	if err := config.RuntimeFingerprint.Validate(); err != nil {
		return err
	}
	derived, err := currentRuntimeFingerprint(config.RatGeneration.Runtime.SourceSHA256)
	if err != nil || derived != config.RuntimeFingerprint {
		return fmt.Errorf("offline matrix runtime fingerprint is not code-derived")
	}
	if err := validateOfflineMatrixEndpoints(config.DatabaseURL, config.OllamaEndpoint); err != nil {
		return err
	}
	if err := validateOfflineOutputDirectories(
		config.PublicOutputDirectory, config.PrivateOutputDirectory,
	); err != nil {
		return err
	}
	for label, value := range map[string]string{
		"Task Ledger schema version":        config.LedgerSchemaVersion,
		"Working Set policy version":        config.WorkingSetPolicyVersion,
		"Context Projection policy version": config.ProjectionPolicyVersion,
	} {
		if err := requireExact(value, label, 256); err != nil {
			return err
		}
	}
	registration, err := LoadOfflineMatrixPreregistration(config.Paths().Preregistration)
	if err != nil {
		return err
	}
	registrationSHA256, err := registration.SHA256()
	if err != nil || registrationSHA256 != config.PreregistrationSHA256 ||
		!registration.Matches(config.Plan, config.fixedAuthority()) {
		return fmt.Errorf("offline matrix preregistration changed")
	}
	return nil
}

func (config OfflineMatrixConfig) ValidateStart() error {
	if err := config.Validate(); err != nil {
		return err
	}
	if _, err := os.Lstat(config.Paths().Receipt); !os.IsNotExist(err) {
		return fmt.Errorf("offline matrix receipt already exists or is inaccessible")
	}
	return nil
}

func (config OfflineMatrixConfig) fixedAuthority() OfflineMatrixFixedAuthority {
	return OfflineMatrixFixedAuthority{
		Budget: config.Budget, RatGeneration: config.RatGeneration,
		RuntimeFingerprint:      config.RuntimeFingerprint,
		InferenceTimeoutSeconds: config.InferenceTimeoutSeconds,
		OmnidexCommit:           config.OmnidexCommit,
		LedgerSchemaVersion:     config.LedgerSchemaVersion,
		WorkingSetPolicyVersion: config.WorkingSetPolicyVersion,
		ProjectionPolicyVersion: config.ProjectionPolicyVersion,
	}
}

func (config OfflineMatrixConfig) Paths() OfflineMatrixPaths {
	return OfflineMatrixPaths{
		Preregistration: filepath.Join(config.PrivateOutputDirectory, "matrix-preregistration.json"),
		Receipt:         filepath.Join(config.PrivateOutputDirectory, "matrix-receipt.json"),
	}
}

func validateOfflineMatrixEndpoints(databaseURL, ollamaEndpoint string) error {
	database, err := url.Parse(databaseURL)
	if err != nil || (database.Scheme != "postgres" && database.Scheme != "postgresql") ||
		database.Host == "" {
		return fmt.Errorf("offline matrix database URL is invalid")
	}
	provider, err := url.Parse(ollamaEndpoint)
	if err != nil || (provider.Scheme != "http" && provider.Scheme != "https") ||
		provider.Host == "" {
		return fmt.Errorf("offline matrix Ollama endpoint is invalid")
	}
	return nil
}

func (config OfflineMatrixConfig) runConfig(
	coordinate OfflineMatrixCase,
	variant Variant,
) (OfflinePromotionConfig, error) {
	run, err := config.derivedRunConfig(coordinate, variant)
	if err != nil {
		return OfflinePromotionConfig{}, err
	}
	if err := createMatrixRunDirectories(
		run.PublicOutputDirectory, run.PrivateOutputDirectory,
	); err != nil {
		return OfflinePromotionConfig{}, err
	}
	return run, run.Validate()
}

func (config OfflineMatrixConfig) derivedRunConfig(
	coordinate OfflineMatrixCase,
	variant Variant,
) (OfflinePromotionConfig, error) {
	scenario, err := ResolveOfflineScenarioSpecV1(
		coordinate.Suite, coordinate.Seed, config.Budget,
	)
	if err != nil {
		return OfflinePromotionConfig{}, err
	}
	if scenario.Initial != nil {
		scenario.Initial.CaseID = coordinate.ID
	} else {
		scenario.Extended.CaseID = coordinate.ID
	}
	publicDirectory := filepath.Join(
		config.PublicOutputDirectory, "runs", coordinate.ID, string(variant),
	)
	privateDirectory := filepath.Join(
		config.PrivateOutputDirectory, "runs", coordinate.ID, string(variant),
	)
	run := OfflinePromotionConfig{
		Schema: OfflinePromotionConfigSchemaV1, DatabaseURL: config.DatabaseURL,
		OllamaEndpoint:          config.OllamaEndpoint,
		InferenceTimeoutSeconds: config.InferenceTimeoutSeconds,
		Scenario:                scenario, Variant: variant, Surface: config.Plan.Surface,
		RatGeneration: config.RatGeneration, RuntimeFingerprint: config.RuntimeFingerprint,
		Repetition: coordinate.Repetition, PublicOutputDirectory: publicDirectory,
		PrivateOutputDirectory: privateDirectory, OmnidexCommit: config.OmnidexCommit,
		LedgerSchemaVersion:     config.LedgerSchemaVersion,
		WorkingSetPolicyVersion: config.WorkingSetPolicyVersion,
		ProjectionPolicyVersion: config.ProjectionPolicyVersion,
	}
	return run, nil
}

func createMatrixRunDirectories(publicDirectory, privateDirectory string) error {
	for _, path := range []string{publicDirectory, privateDirectory} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			return fmt.Errorf("offline matrix run directory already exists or is inaccessible")
		}
	}
	for _, directory := range []struct {
		path string
		mode os.FileMode
	}{{filepath.Dir(publicDirectory), 0o755}, {filepath.Dir(privateDirectory), 0o700}} {
		if err := os.MkdirAll(directory.path, directory.mode); err != nil {
			return err
		}
	}
	if err := os.Mkdir(publicDirectory, 0o755); err != nil {
		return err
	}
	if err := os.Mkdir(privateDirectory, 0o700); err != nil {
		_ = os.Remove(publicDirectory)
		return err
	}
	if err := validateOfflineOutputDirectories(publicDirectory, privateDirectory); err != nil {
		_ = os.Remove(privateDirectory)
		_ = os.Remove(publicDirectory)
		return err
	}
	for _, path := range []string{publicDirectory, privateDirectory} {
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("offline matrix run directory disappeared during creation")
		}
	}
	return nil
}

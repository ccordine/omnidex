package cognitiongauntlet

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	OfflineTransferRequestSchemaV1 = "omnidex.offline-transfer-request.v1"
	OfflineTransferConfigSchemaV1  = "omnidex.offline-transfer-config.v1"
)

type OfflineTransferRequest struct {
	Schema                  string              `json:"schema"`
	Plan                    OfflineTransferPlan `json:"plan"`
	Budget                  RunBudget           `json:"budget"`
	DatabaseURL             string              `json:"database_url"`
	OllamaEndpoint          string              `json:"ollama_endpoint"`
	InferenceTimeoutSeconds int                 `json:"inference_timeout_seconds"`
	PublicOutputDirectory   string              `json:"public_output_directory"`
	PrivateOutputDirectory  string              `json:"private_output_directory"`
	Brain                   OfflineBrainRequest `json:"brain"`
}

type OfflineTransferConfig struct {
	Schema                  string              `json:"schema"`
	Plan                    OfflineTransferPlan `json:"plan"`
	Budget                  RunBudget           `json:"budget"`
	DatabaseURL             string              `json:"database_url"`
	OllamaEndpoint          string              `json:"ollama_endpoint"`
	InferenceTimeoutSeconds int                 `json:"inference_timeout_seconds"`
	PublicOutputDirectory   string              `json:"public_output_directory"`
	PrivateOutputDirectory  string              `json:"private_output_directory"`
	RatGeneration           RatGeneration       `json:"rat_generation"`
	RuntimeFingerprint      RuntimeFingerprint  `json:"runtime_fingerprint"`
	PreregistrationSHA256   string              `json:"preregistration_sha256"`
	OmnidexCommit           string              `json:"omnidex_commit"`
	LedgerSchemaVersion     string              `json:"ledger_schema_version"`
	WorkingSetPolicyVersion string              `json:"working_set_policy_version"`
	ProjectionPolicyVersion string              `json:"projection_policy_version"`
}

type OfflineTransferPaths struct {
	Preregistration string
	Receipt         string
}

func (request OfflineTransferRequest) Validate() error {
	if request.Schema != OfflineTransferRequestSchemaV1 || request.Plan.Validate() != nil ||
		request.Budget.Schema != RunBudgetSchemaStructuralV1 || request.Budget.Validate() != nil ||
		request.Brain.NativeContextLimit <= 0 || request.InferenceTimeoutSeconds <= 0 ||
		request.InferenceTimeoutSeconds > 24*60*60 {
		return fmt.Errorf("offline Transfer request is invalid")
	}
	if err := requireExact(request.Brain.Model, "offline Transfer model", 512); err != nil {
		return err
	}
	if err := validateOfflineMatrixEndpoints(request.DatabaseURL, request.OllamaEndpoint); err != nil {
		return err
	}
	if _, err := ResolveOfflineScenarioSpecV1(
		request.Plan.Suite, request.Plan.Seed, request.Budget,
	); err != nil {
		return err
	}
	return validateOfflineOutputDirectories(
		request.PublicOutputDirectory, request.PrivateOutputDirectory,
	)
}

func (config OfflineTransferConfig) Validate() error {
	if config.Schema != OfflineTransferConfigSchemaV1 || config.Plan.Validate() != nil ||
		!validDigest(config.PreregistrationSHA256) || !validCommitIdentity(config.OmnidexCommit) ||
		config.InferenceTimeoutSeconds <= 0 || config.InferenceTimeoutSeconds > 24*60*60 {
		return fmt.Errorf("offline Transfer configuration is invalid")
	}
	if err := config.fixedAuthority().Validate(); err != nil {
		return err
	}
	derived, err := currentRuntimeFingerprint(config.RatGeneration.Runtime.SourceSHA256)
	if err != nil || derived != config.RuntimeFingerprint {
		return fmt.Errorf("offline Transfer runtime fingerprint is not code-derived")
	}
	if err := validateOfflineMatrixEndpoints(config.DatabaseURL, config.OllamaEndpoint); err != nil {
		return err
	}
	if err := validateOfflineOutputDirectories(
		config.PublicOutputDirectory, config.PrivateOutputDirectory,
	); err != nil {
		return err
	}
	registration, err := LoadOfflineTransferPreregistration(config.Paths().Preregistration)
	if err != nil {
		return err
	}
	registrationSHA, err := registration.SHA256()
	if err != nil || registrationSHA != config.PreregistrationSHA256 ||
		!registration.Matches(config.Plan, config.fixedAuthority()) {
		return fmt.Errorf("offline Transfer preregistration changed")
	}
	return nil
}

func (config OfflineTransferConfig) ValidateStart() error {
	if err := config.Validate(); err != nil {
		return err
	}
	if _, err := os.Lstat(config.Paths().Receipt); !os.IsNotExist(err) {
		return fmt.Errorf("offline Transfer receipt already exists or is inaccessible")
	}
	return nil
}

func (config OfflineTransferConfig) fixedAuthority() OfflineMatrixFixedAuthority {
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

func (config OfflineTransferConfig) Paths() OfflineTransferPaths {
	return OfflineTransferPaths{
		Preregistration: filepath.Join(config.PrivateOutputDirectory, "transfer-preregistration.json"),
		Receipt:         filepath.Join(config.PrivateOutputDirectory, "transfer-receipt.json"),
	}
}

func (config OfflineTransferConfig) derivedRunConfig(
	registration OfflineTransferPreregistration,
	surface Surface,
) (OfflinePromotionConfig, error) {
	if transferSurfaceRank(surface) == 0 || !containsSurface(registration.Plan.Surfaces, surface) {
		return OfflinePromotionConfig{}, fmt.Errorf("offline Transfer surface is not preregistered")
	}
	publicDirectory := filepath.Join(config.PublicOutputDirectory, "runs", string(surface))
	privateDirectory := filepath.Join(config.PrivateOutputDirectory, "runs", string(surface))
	return OfflinePromotionConfig{
		Schema: OfflinePromotionConfigSchemaV1, DatabaseURL: config.DatabaseURL,
		OllamaEndpoint:          config.OllamaEndpoint,
		InferenceTimeoutSeconds: config.InferenceTimeoutSeconds,
		Scenario:                registration.Workload, Variant: VariantFullCognition, Surface: surface,
		RatGeneration: config.RatGeneration, RuntimeFingerprint: config.RuntimeFingerprint,
		Repetition: config.Plan.Repetition, PublicOutputDirectory: publicDirectory,
		PrivateOutputDirectory: privateDirectory, OmnidexCommit: config.OmnidexCommit,
		LedgerSchemaVersion:     config.LedgerSchemaVersion,
		WorkingSetPolicyVersion: config.WorkingSetPolicyVersion,
		ProjectionPolicyVersion: config.ProjectionPolicyVersion,
	}, nil
}

func (config OfflineTransferConfig) runConfig(
	registration OfflineTransferPreregistration,
	surface Surface,
) (OfflinePromotionConfig, error) {
	run, err := config.derivedRunConfig(registration, surface)
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

func containsSurface(values []Surface, expected Surface) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

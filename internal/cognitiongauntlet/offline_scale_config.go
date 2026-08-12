package cognitiongauntlet

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	OfflineScaleRequestSchemaV1 = "omnidex.offline-scale-request.v1"
	OfflineScaleConfigSchemaV2  = "omnidex.offline-scale-config.v2"
)

type OfflineScaleRequest struct {
	Schema                  string              `json:"schema"`
	Plan                    OfflineScalePlan    `json:"plan"`
	Budget                  RunBudget           `json:"budget"`
	DatabaseURL             string              `json:"database_url"`
	OllamaEndpoint          string              `json:"ollama_endpoint"`
	InferenceTimeoutSeconds int                 `json:"inference_timeout_seconds"`
	PublicOutputDirectory   string              `json:"public_output_directory"`
	PrivateOutputDirectory  string              `json:"private_output_directory"`
	Brain                   OfflineBrainRequest `json:"brain"`
}

type OfflineScaleConfig struct {
	Schema                  string                         `json:"schema"`
	Plan                    OfflineScalePlan               `json:"plan"`
	Budget                  RunBudget                      `json:"budget"`
	DatabaseURL             string                         `json:"database_url"`
	OllamaEndpoint          string                         `json:"ollama_endpoint"`
	InferenceTimeoutSeconds int                            `json:"inference_timeout_seconds"`
	PublicOutputDirectory   string                         `json:"public_output_directory"`
	PrivateOutputDirectory  string                         `json:"private_output_directory"`
	RatGeneration           RatGeneration                  `json:"rat_generation"`
	PreparedBrainEvidence   PreparedBrainEvidenceAuthority `json:"prepared_brain_evidence"`
	RuntimeFingerprint      RuntimeFingerprint             `json:"runtime_fingerprint"`
	PreregistrationSHA256   string                         `json:"preregistration_sha256"`
	OmnidexCommit           string                         `json:"omnidex_commit"`
	LedgerSchemaVersion     string                         `json:"ledger_schema_version"`
	WorkingSetPolicyVersion string                         `json:"working_set_policy_version"`
	ProjectionPolicyVersion string                         `json:"projection_policy_version"`
}

type OfflineScalePaths struct {
	Preregistration string
	Receipt         string
}

func (request OfflineScaleRequest) Validate() error {
	if request.Schema != OfflineScaleRequestSchemaV1 || request.Plan.Validate() != nil ||
		request.Budget.Schema != RunBudgetSchemaStructuralV1 || request.Budget.Validate() != nil ||
		request.Brain.NativeContextLimit <= 0 || request.InferenceTimeoutSeconds <= 0 ||
		request.InferenceTimeoutSeconds > 24*60*60 {
		return fmt.Errorf("offline Scale request is invalid")
	}
	if err := requireExact(request.Brain.Model, "offline Scale model", 512); err != nil {
		return err
	}
	if err := validateOfflineMatrixEndpoints(request.DatabaseURL, request.OllamaEndpoint); err != nil {
		return err
	}
	if _, err := ResolveOfflineScenarioSpecV1(SuiteCombined, request.Plan.Seed, request.Budget); err != nil {
		return err
	}
	return validateOfflineOutputDirectories(
		request.PublicOutputDirectory, request.PrivateOutputDirectory,
	)
}

func (config OfflineScaleConfig) Validate() error {
	if config.Schema != OfflineScaleConfigSchemaV2 || config.Plan.Validate() != nil ||
		!validDigest(config.PreregistrationSHA256) || !validCommitIdentity(config.OmnidexCommit) ||
		config.InferenceTimeoutSeconds <= 0 || config.InferenceTimeoutSeconds > 24*60*60 {
		return fmt.Errorf("offline Scale configuration is invalid")
	}
	if err := config.fixedAuthority().Validate(); err != nil {
		return err
	}
	derived, err := currentRuntimeFingerprint(config.RatGeneration.Runtime.SourceSHA256)
	if err != nil || derived != config.RuntimeFingerprint {
		return fmt.Errorf("offline Scale runtime fingerprint is not code-derived")
	}
	if err := validateOfflineMatrixEndpoints(config.DatabaseURL, config.OllamaEndpoint); err != nil {
		return err
	}
	if err := validateOfflineOutputDirectories(
		config.PublicOutputDirectory, config.PrivateOutputDirectory,
	); err != nil {
		return err
	}
	registration, err := LoadOfflineScalePreregistration(config.Paths().Preregistration)
	if err != nil {
		return err
	}
	sha, err := registration.SHA256()
	if err != nil || sha != config.PreregistrationSHA256 ||
		!registration.Matches(config.Plan, config.fixedAuthority()) {
		return fmt.Errorf("offline Scale preregistration changed")
	}
	return nil
}

func (config OfflineScaleConfig) ValidateStart() error {
	if err := config.Validate(); err != nil {
		return err
	}
	if _, err := os.Lstat(config.Paths().Receipt); !os.IsNotExist(err) {
		return fmt.Errorf("offline Scale receipt already exists or is inaccessible")
	}
	registration, err := LoadOfflineScalePreregistration(config.Paths().Preregistration)
	if err != nil {
		return err
	}
	for _, current := range registration.Cases {
		if err := validateOfflineOutputTargets(config.runPaths(current)); err != nil {
			return err
		}
		if _, err := os.Lstat(config.scaleEvidencePath(current)); !os.IsNotExist(err) {
			return fmt.Errorf("offline Scale evidence output already exists or is inaccessible")
		}
	}
	return nil
}

func (config OfflineScaleConfig) fixedAuthority() OfflineMatrixFixedAuthority {
	return OfflineMatrixFixedAuthority{
		Budget: config.Budget, RatGeneration: config.RatGeneration,
		PreparedBrainEvidence:   config.PreparedBrainEvidence,
		RuntimeFingerprint:      config.RuntimeFingerprint,
		InferenceTimeoutSeconds: config.InferenceTimeoutSeconds,
		OmnidexCommit:           config.OmnidexCommit,
		LedgerSchemaVersion:     config.LedgerSchemaVersion,
		WorkingSetPolicyVersion: config.WorkingSetPolicyVersion,
		ProjectionPolicyVersion: config.ProjectionPolicyVersion,
	}
}

func (config OfflineScaleConfig) Paths() OfflineScalePaths {
	return OfflineScalePaths{
		Preregistration: filepath.Join(config.PrivateOutputDirectory, "scale-preregistration.json"),
		Receipt:         filepath.Join(config.PrivateOutputDirectory, "scale-receipt.json"),
	}
}

func (config OfflineScaleConfig) executionAuthority(current OfflineScaleCase) offlineExecutionAuthority {
	return offlineExecutionAuthority{
		DatabaseURL: config.DatabaseURL, OllamaEndpoint: config.OllamaEndpoint,
		InferenceTimeoutSeconds: config.InferenceTimeoutSeconds,
		Variant:                 VariantFullCognition, RatGeneration: config.RatGeneration,
		PreparedBrainEvidence: config.PreparedBrainEvidence,
		RuntimeFingerprint:    config.RuntimeFingerprint, Budget: config.Budget,
		Repetition: current.Repetition, OmnidexCommit: config.OmnidexCommit,
		LedgerSchemaVersion:     config.LedgerSchemaVersion,
		WorkingSetPolicyVersion: config.WorkingSetPolicyVersion,
		ProjectionPolicyVersion: config.ProjectionPolicyVersion,
	}
}

func (config OfflineScaleConfig) runPaths(current OfflineScaleCase) OfflinePromotionPaths {
	publicDirectory := filepath.Join(config.PublicOutputDirectory, "runs", current.ID)
	privateDirectory := filepath.Join(config.PrivateOutputDirectory, "runs", current.ID)
	return OfflinePromotionPaths{
		PublicBundle:  filepath.Join(publicDirectory, "inference-bootstrap.json"),
		Episode:       filepath.Join(publicDirectory, "sealed-episode.json"),
		Evidence:      filepath.Join(publicDirectory, "ablation-evidence.json"),
		PrivateOracle: filepath.Join(privateDirectory, "private-oracle.json"),
		Evaluation:    filepath.Join(privateDirectory, "evaluation.json"),
		Receipt:       filepath.Join(privateDirectory, "promotion-receipt.json"),
	}
}

func (config OfflineScaleConfig) scaleEvidencePath(current OfflineScaleCase) string {
	return filepath.Join(
		config.PrivateOutputDirectory, "runs", current.ID, "scale-evidence.json",
	)
}

func (config OfflineScaleConfig) createRunDirectories(current OfflineScaleCase) error {
	paths := config.runPaths(current)
	return createMatrixRunDirectories(filepath.Dir(paths.PublicBundle), filepath.Dir(paths.PrivateOracle))
}

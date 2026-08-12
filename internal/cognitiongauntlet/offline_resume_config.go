package cognitiongauntlet

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	OfflineResumeRequestSchemaV1 = "omnidex.offline-resume-request.v1"
	OfflineResumeConfigSchemaV2  = "omnidex.offline-resume-config.v2"
)

type OfflineResumeRequest struct {
	Schema                  string              `json:"schema"`
	Plan                    OfflineResumePlan   `json:"plan"`
	Budget                  RunBudget           `json:"budget"`
	DatabaseURL             string              `json:"database_url"`
	OllamaEndpoint          string              `json:"ollama_endpoint"`
	InferenceTimeoutSeconds int                 `json:"inference_timeout_seconds"`
	PublicOutputDirectory   string              `json:"public_output_directory"`
	PrivateOutputDirectory  string              `json:"private_output_directory"`
	Brain                   OfflineBrainRequest `json:"brain"`
}

type OfflineResumeConfig struct {
	Schema                  string             `json:"schema"`
	Plan                    OfflineResumePlan  `json:"plan"`
	Budget                  RunBudget          `json:"budget"`
	DatabaseURL             string             `json:"database_url"`
	OllamaEndpoint          string             `json:"ollama_endpoint"`
	InferenceTimeoutSeconds int                `json:"inference_timeout_seconds"`
	PublicOutputDirectory   string             `json:"public_output_directory"`
	PrivateOutputDirectory  string             `json:"private_output_directory"`
	RatGeneration           RatGeneration      `json:"rat_generation"`
	PreparedBrainEvidence   PreparedBrainEvidenceAuthority `json:"prepared_brain_evidence"`
	RuntimeFingerprint      RuntimeFingerprint `json:"runtime_fingerprint"`
	PreregistrationSHA256   string             `json:"preregistration_sha256"`
	OmnidexCommit           string             `json:"omnidex_commit"`
	LedgerSchemaVersion     string             `json:"ledger_schema_version"`
	WorkingSetPolicyVersion string             `json:"working_set_policy_version"`
	ProjectionPolicyVersion string             `json:"projection_policy_version"`
}

type OfflineResumePaths struct {
	Preregistration string
	Receipt         string
}

func (request OfflineResumeRequest) Validate() error {
	if request.Schema != OfflineResumeRequestSchemaV1 || request.Plan.Validate() != nil ||
		request.Budget.Validate() != nil || request.Brain.NativeContextLimit <= 0 ||
		request.InferenceTimeoutSeconds <= 0 || request.InferenceTimeoutSeconds > 24*60*60 {
		return fmt.Errorf("offline Resume request is invalid")
	}
	if request.Budget.Schema != RunBudgetSchemaStructuralV1 {
		return fmt.Errorf("offline Resume request requires the structural v1 budget authority")
	}
	if err := requireExact(request.Brain.Model, "offline Resume model", 512); err != nil {
		return err
	}
	if err := validateOfflineMatrixEndpoints(request.DatabaseURL, request.OllamaEndpoint); err != nil {
		return err
	}
	if _, err := resumeWorkloadSpec(request.Plan, request.Budget); err != nil {
		return err
	}
	return validateOfflineOutputDirectories(
		request.PublicOutputDirectory, request.PrivateOutputDirectory,
	)
}

func (config OfflineResumeConfig) Validate() error {
	if config.Schema != OfflineResumeConfigSchemaV2 || config.Plan.Validate() != nil ||
		!validDigest(config.PreregistrationSHA256) || !validCommitIdentity(config.OmnidexCommit) ||
		config.InferenceTimeoutSeconds <= 0 || config.InferenceTimeoutSeconds > 24*60*60 {
		return fmt.Errorf("offline Resume configuration is invalid")
	}
	if err := config.fixedAuthority().Validate(); err != nil {
		return err
	}
	derived, err := currentRuntimeFingerprint(config.RatGeneration.Runtime.SourceSHA256)
	if err != nil || derived != config.RuntimeFingerprint {
		return fmt.Errorf("offline Resume runtime fingerprint is not code-derived")
	}
	if err := validateOfflineMatrixEndpoints(config.DatabaseURL, config.OllamaEndpoint); err != nil {
		return err
	}
	if err := validateOfflineOutputDirectories(
		config.PublicOutputDirectory, config.PrivateOutputDirectory,
	); err != nil {
		return err
	}
	registration, err := LoadOfflineResumePreregistration(config.Paths().Preregistration)
	if err != nil {
		return err
	}
	registrationSHA, err := registration.SHA256()
	if err != nil || registrationSHA != config.PreregistrationSHA256 ||
		!registration.Matches(config.Plan, config.fixedAuthority()) {
		return fmt.Errorf("offline Resume preregistration changed")
	}
	return nil
}

func (config OfflineResumeConfig) ValidateStart() error {
	if err := config.Validate(); err != nil {
		return err
	}
	if _, err := os.Lstat(config.Paths().Receipt); !os.IsNotExist(err) {
		return fmt.Errorf("offline Resume receipt already exists or is inaccessible")
	}
	return nil
}

func (config OfflineResumeConfig) fixedAuthority() OfflineMatrixFixedAuthority {
	return OfflineMatrixFixedAuthority{
		Budget: config.Budget, RatGeneration: config.RatGeneration,
		PreparedBrainEvidence: config.PreparedBrainEvidence,
		RuntimeFingerprint:      config.RuntimeFingerprint,
		InferenceTimeoutSeconds: config.InferenceTimeoutSeconds,
		OmnidexCommit:           config.OmnidexCommit,
		LedgerSchemaVersion:     config.LedgerSchemaVersion,
		WorkingSetPolicyVersion: config.WorkingSetPolicyVersion,
		ProjectionPolicyVersion: config.ProjectionPolicyVersion,
	}
}

func (config OfflineResumeConfig) Paths() OfflineResumePaths {
	return OfflineResumePaths{
		Preregistration: filepath.Join(config.PrivateOutputDirectory, "resume-preregistration.json"),
		Receipt:         filepath.Join(config.PrivateOutputDirectory, "resume-receipt.json"),
	}
}

func (config OfflineResumeConfig) derivedRunConfig(
	registration OfflineResumePreregistration,
	schedule OfflineResumeSchedule,
) (OfflinePromotionConfig, error) {
	if err := schedule.Validate(
		registration.Workload.Initial.Generator.Difficulty.SolutionDepth,
		config.Budget.ModelCalls,
	); err != nil {
		return OfflinePromotionConfig{}, err
	}
	publicDirectory := filepath.Join(config.PublicOutputDirectory, "runs", schedule.ID)
	privateDirectory := filepath.Join(config.PrivateOutputDirectory, "runs", schedule.ID)
	return OfflinePromotionConfig{
		Schema: OfflinePromotionConfigSchemaV2, DatabaseURL: config.DatabaseURL,
		OllamaEndpoint:          config.OllamaEndpoint,
		InferenceTimeoutSeconds: config.InferenceTimeoutSeconds,
		Scenario:                registration.Workload, Variant: VariantFullCognition,
		Surface: config.Plan.Surface, RatGeneration: config.RatGeneration,
		PreparedBrainEvidence: config.PreparedBrainEvidence,
		RuntimeFingerprint: config.RuntimeFingerprint, Repetition: config.Plan.Repetition,
		PublicOutputDirectory: publicDirectory, PrivateOutputDirectory: privateDirectory,
		OmnidexCommit: config.OmnidexCommit, LedgerSchemaVersion: config.LedgerSchemaVersion,
		WorkingSetPolicyVersion: config.WorkingSetPolicyVersion,
		ProjectionPolicyVersion: config.ProjectionPolicyVersion,
	}, nil
}

func (config OfflineResumeConfig) runConfig(
	registration OfflineResumePreregistration,
	schedule OfflineResumeSchedule,
) (OfflinePromotionConfig, error) {
	run, err := config.derivedRunConfig(registration, schedule)
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

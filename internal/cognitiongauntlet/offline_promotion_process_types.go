package cognitiongauntlet

import (
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/model"
)

const (
	generatorProcessConfigSchemaV1  = "omnidex.offline-cognition-generator-process.v1"
	hostProcessConfigSchemaV1       = "omnidex.offline-cognition-host-process.v1"
	hostProcessReadySchemaV1        = "omnidex.offline-cognition-host-ready.v1"
	inferenceProcessConfigSchemaV1  = "omnidex.offline-cognition-inference-process.v1"
	evaluatorProcessConfigSchemaV1  = "omnidex.offline-cognition-evaluator-process.v1"
	OfflinePromotionReceiptSchemaV1 = "omnidex.offline-cognition-promotion-receipt.v1"
)

type generatorProcessConfig struct {
	Schema                  string              `json:"schema"`
	Scenario                OfflineScenarioSpec `json:"scenario"`
	Variant                 Variant             `json:"variant"`
	Surface                 Surface             `json:"surface"`
	RatGeneration           RatGeneration       `json:"rat_generation"`
	RuntimeFingerprint      RuntimeFingerprint  `json:"runtime_fingerprint"`
	Repetition              int                 `json:"repetition"`
	PublicBundlePath        string              `json:"public_bundle_path"`
	HostScenarioPath        string              `json:"host_scenario_path"`
	PrivateOraclePath       string              `json:"private_oracle_path"`
	PrivateOracleCredential string              `json:"private_oracle_credential"`
	ExecutableSHA256        string              `json:"executable_sha256"`
	SourceSHA256            string              `json:"source_sha256"`
	OmnidexCommit           string              `json:"omnidex_commit"`
}

type hostProcessConfig struct {
	Schema           string                `json:"schema"`
	DatabaseURL      string                `json:"database_url"`
	DatabaseSchema   string                `json:"database_schema"`
	HostSchema       string                `json:"host_schema"`
	ExpectedRole     string                `json:"expected_role"`
	Scenario         cognition.ScenarioRef `json:"scenario"`
	HostScenarioPath string                `json:"host_scenario_path"`
	PublicBundlePath string                `json:"public_bundle_path"`
	ReadyPath        string                `json:"ready_path"`
	EnvironmentToken string                `json:"environment_token"`
	ExecutableSHA256 string                `json:"executable_sha256"`
	SourceSHA256     string                `json:"source_sha256"`
	OmnidexCommit    string                `json:"omnidex_commit"`
}

type hostProcessReady struct {
	Schema      string                `json:"schema"`
	PID         int                   `json:"pid"`
	BaseURL     string                `json:"base_url"`
	CurrentRole string                `json:"current_role"`
	Scenario    cognition.ScenarioRef `json:"scenario"`
	StartedAt   time.Time             `json:"started_at"`
}

type inferenceProcessConfig struct {
	Schema                  string                     `json:"schema"`
	DatabaseURL             string                     `json:"database_url"`
	DatabaseSchema          string                     `json:"database_schema"`
	EnvironmentURL          string                     `json:"environment_url"`
	EnvironmentToken        string                     `json:"environment_token"`
	OllamaEndpoint          string                     `json:"ollama_endpoint"`
	TimeoutSeconds          int                        `json:"timeout_seconds"`
	PublicBundlePath        string                     `json:"public_bundle_path"`
	EpisodePath             string                     `json:"episode_path"`
	ContaminatedOraclePath  string                     `json:"contaminated_oracle_path,omitempty"`
	ContaminatedOracleGrant string                     `json:"contaminated_oracle_grant,omitempty"`
	Attempt                 model.StepAttemptAuthority `json:"attempt"`
	ExecutableSHA256        string                     `json:"executable_sha256"`
	SourceSHA256            string                     `json:"source_sha256"`
	OmnidexCommit           string                     `json:"omnidex_commit"`
	LedgerSchemaVersion     string                     `json:"ledger_schema_version"`
	WorkingSetPolicyVersion string                     `json:"working_set_policy_version"`
	ProjectionPolicyVersion string                     `json:"projection_policy_version"`
	Control                 inferenceProcessControl    `json:"control"`
}

type evaluatorProcessConfig struct {
	Schema                  string `json:"schema"`
	PrivateOraclePath       string `json:"private_oracle_path"`
	PrivateOracleCredential string `json:"private_oracle_credential"`
	PublicBundlePath        string `json:"public_bundle_path"`
	EpisodePath             string `json:"episode_path"`
	EvaluationPath          string `json:"evaluation_path"`
	ExecutableSHA256        string `json:"executable_sha256"`
	SourceSHA256            string `json:"source_sha256"`
	OmnidexCommit           string `json:"omnidex_commit"`
}

type OfflinePromotionReceipt struct {
	Schema                   string             `json:"schema"`
	PublicRunAuthoritySHA256 string             `json:"public_run_authority_sha256"`
	EpisodeSealSHA256        string             `json:"episode_seal_sha256"`
	EvaluationOracleSHA256   string             `json:"evaluation_oracle_sha256"`
	EvaluationArtifactSHA256 string             `json:"evaluation_artifact_sha256"`
	ExecutableSHA256         string             `json:"executable_sha256"`
	SourceSHA256             string             `json:"source_sha256"`
	MigrationsSHA256         string             `json:"migrations_sha256"`
	RuntimeVersion           string             `json:"runtime_version"`
	OmnidexCommit            string             `json:"omnidex_commit"`
	DatabaseSchema           string             `json:"database_schema"`
	GeneratorPID             int                `json:"generator_pid"`
	GeneratorExitedAt        time.Time          `json:"generator_exited_at"`
	Host                     OfflineHostReceipt `json:"host"`
	InferencePID             int                `json:"inference_pid"`
	InferenceStartedAt       time.Time          `json:"inference_started_at"`
	InferenceExitedAt        time.Time          `json:"inference_exited_at"`
	EvaluatorPID             int                `json:"evaluator_pid"`
	EvaluatorStartedAt       time.Time          `json:"evaluator_started_at"`
	CompletedAt              time.Time          `json:"completed_at"`
}

func (receipt OfflinePromotionReceipt) Validate() error {
	if receipt.Schema != OfflinePromotionReceiptSchemaV1 ||
		!validDigest(receipt.PublicRunAuthoritySHA256) || !validDigest(receipt.EpisodeSealSHA256) ||
		!validDigest(receipt.EvaluationOracleSHA256) || !validDigest(receipt.ExecutableSHA256) ||
		!validDigest(receipt.EvaluationArtifactSHA256) ||
		!validDigest(receipt.SourceSHA256) || !validDigest(receipt.MigrationsSHA256) ||
		requireExact(receipt.RuntimeVersion, "promotion receipt runtime version", 256) != nil ||
		!validCommitIdentity(receipt.OmnidexCommit) ||
		receipt.GeneratorPID <= 0 || receipt.InferencePID <= 0 || receipt.EvaluatorPID <= 0 ||
		receipt.Host.validateChronology(
			receipt.GeneratorExitedAt, receipt.InferenceExitedAt, receipt.EvaluatorStartedAt,
		) != nil || receipt.InferenceStartedAt.Before(receipt.Host.StartedAt) ||
		receipt.InferenceExitedAt.Before(receipt.InferenceStartedAt) ||
		receipt.Host.PID == receipt.GeneratorPID ||
		receipt.Host.PID == receipt.InferencePID || receipt.Host.PID == receipt.EvaluatorPID ||
		receipt.DatabaseSchema == "" || receipt.GeneratorExitedAt.IsZero() ||
		receipt.InferenceExitedAt.Before(receipt.GeneratorExitedAt) || receipt.EvaluatorStartedAt.IsZero() ||
		receipt.CompletedAt.IsZero() || receipt.EvaluatorStartedAt.Before(receipt.InferenceExitedAt) ||
		receipt.CompletedAt.Before(receipt.EvaluatorStartedAt) {
		return fmt.Errorf("offline cognition promotion receipt is invalid")
	}
	return nil
}

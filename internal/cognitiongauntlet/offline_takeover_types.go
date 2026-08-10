package cognitiongauntlet

import (
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

const (
	OfflineTakeoverConfigSchemaV1  = "omnidex.offline-cognition-takeover-config.v1"
	OfflineTakeoverReceiptSchemaV1 = "omnidex.offline-cognition-takeover-receipt.v1"
)

type OfflineTakeoverConfig struct {
	Schema                 string                 `json:"schema"`
	Promotion              OfflinePromotionConfig `json:"promotion"`
	AfterSuccessfulActions uint32                 `json:"after_successful_actions"`
}

type OfflineTakeoverReceipt struct {
	Schema                   string                     `json:"schema"`
	PublicRunAuthoritySHA256 string                     `json:"public_run_authority_sha256"`
	EpisodeSealSHA256        string                     `json:"episode_seal_sha256"`
	EvaluationOracleSHA256   string                     `json:"evaluation_oracle_sha256"`
	EvaluationArtifactSHA256 string                     `json:"evaluation_artifact_sha256"`
	ExecutableSHA256         string                     `json:"executable_sha256"`
	SourceSHA256             string                     `json:"source_sha256"`
	MigrationsSHA256         string                     `json:"migrations_sha256"`
	RuntimeVersion           string                     `json:"runtime_version"`
	OmnidexCommit            string                     `json:"omnidex_commit"`
	Original                 model.StepAttemptAuthority `json:"original_attempt"`
	Replacement              model.StepAttemptAuthority `json:"replacement_attempt"`
	GeneratorPID             int                        `json:"generator_pid"`
	OriginalPID              int                        `json:"original_pid"`
	ReplacementPID           int                        `json:"replacement_pid"`
	EvaluatorPID             int                        `json:"evaluator_pid"`
	Host                     OfflineHostReceipt         `json:"host"`
	GeneratorExitedAt        time.Time                  `json:"generator_exited_at"`
	OriginalKilledAt         time.Time                  `json:"original_killed_at"`
	ReplacementExitedAt      time.Time                  `json:"replacement_exited_at"`
	EvaluatorStartedAt       time.Time                  `json:"evaluator_started_at"`
	CompletedAt              time.Time                  `json:"completed_at"`
	Continuity               TakeoverContinuityProof    `json:"continuity"`
}

func (config OfflineTakeoverConfig) Validate() error {
	if config.Schema != OfflineTakeoverConfigSchemaV1 {
		return fmt.Errorf("offline cognition takeover configuration schema is invalid")
	}
	if err := config.Promotion.Validate(); err != nil {
		return err
	}
	if config.Promotion.Variant != VariantFullCognition {
		return fmt.Errorf("offline cognition takeover requires full cognition")
	}
	if config.AfterSuccessfulActions == 0 ||
		int(config.AfterSuccessfulActions) >= config.Promotion.Scenario.Budget().EnvironmentActions {
		return fmt.Errorf("offline cognition takeover boundary is outside the frozen action budget")
	}
	return nil
}

func (receipt OfflineTakeoverReceipt) Validate() error {
	if receipt.Schema != OfflineTakeoverReceiptSchemaV1 ||
		!validDigest(receipt.PublicRunAuthoritySHA256) || !validDigest(receipt.EpisodeSealSHA256) ||
		!validDigest(receipt.EvaluationOracleSHA256) || !validDigest(receipt.ExecutableSHA256) ||
		!validDigest(receipt.EvaluationArtifactSHA256) ||
		!validDigest(receipt.SourceSHA256) || !validDigest(receipt.MigrationsSHA256) ||
		requireExact(receipt.RuntimeVersion, "takeover receipt runtime version", 256) != nil ||
		!validCommitIdentity(receipt.OmnidexCommit) ||
		receipt.GeneratorPID <= 0 || receipt.OriginalPID <= 0 ||
		receipt.ReplacementPID <= 0 || receipt.EvaluatorPID <= 0 ||
		receipt.Host.validateChronology(
			receipt.GeneratorExitedAt, receipt.ReplacementExitedAt, receipt.EvaluatorStartedAt,
		) != nil || receipt.Host.PID == receipt.GeneratorPID ||
		receipt.Host.PID == receipt.OriginalPID || receipt.Host.PID == receipt.ReplacementPID ||
		receipt.Host.PID == receipt.EvaluatorPID ||
		!validTakeoverAttempt(receipt.Original) || !validTakeoverAttempt(receipt.Replacement) ||
		receipt.Replacement.JobID != receipt.Original.JobID ||
		receipt.Replacement.Generation != receipt.Original.Generation ||
		receipt.Replacement.StepID != receipt.Original.StepID ||
		receipt.Replacement.Attempt != receipt.Original.Attempt+1 ||
		receipt.Replacement.WorkerID == receipt.Original.WorkerID ||
		receipt.Continuity.Validate() != nil || receipt.GeneratorExitedAt.IsZero() ||
		receipt.OriginalKilledAt.Before(receipt.GeneratorExitedAt) ||
		receipt.ReplacementExitedAt.Before(receipt.OriginalKilledAt) ||
		receipt.EvaluatorStartedAt.Before(receipt.ReplacementExitedAt) ||
		receipt.CompletedAt.Before(receipt.EvaluatorStartedAt) {
		return fmt.Errorf("offline cognition takeover receipt is invalid")
	}
	return nil
}

func validTakeoverAttempt(authority model.StepAttemptAuthority) bool {
	return authority.JobID > 0 && authority.Generation > 0 && authority.StepID > 0 &&
		authority.Attempt > 0 && authority.WorkerID != ""
}

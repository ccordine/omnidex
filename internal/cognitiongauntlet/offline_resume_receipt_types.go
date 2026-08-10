package cognitiongauntlet

import (
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

const OfflineResumeReceiptSchemaV1 = "omnidex.offline-resume-receipt.v1"

type OfflineResumeInterruptionReceipt struct {
	DecisionBoundary         uint32                     `json:"decision_boundary"`
	BaselineCheckpointSHA256 string                     `json:"baseline_checkpoint_sha256"`
	Original                 model.StepAttemptAuthority `json:"original_attempt"`
	Replacement              model.StepAttemptAuthority `json:"replacement_attempt"`
	OriginalPID              int                        `json:"original_pid"`
	ReplacementPID           int                        `json:"replacement_pid"`
	OriginalDiedAt           time.Time                  `json:"original_died_at"`
	OriginalStoppedAt        time.Time                  `json:"original_stopped_at"`
	OriginalResumedAt        time.Time                  `json:"original_resumed_at"`
	OriginalExitedAt         time.Time                  `json:"original_exited_at"`
	Continuity               TakeoverContinuityProof    `json:"continuity"`
}

type OfflineResumeRunReceipt struct {
	Schedule                 OfflineResumeSchedule              `json:"schedule"`
	ScheduleEvidenceSHA256   string                             `json:"schedule_evidence_sha256"`
	PromotionReceiptSHA256   string                             `json:"promotion_receipt_sha256"`
	PublicRunAuthoritySHA256 string                             `json:"public_run_authority_sha256"`
	EpisodeSealSHA256        string                             `json:"episode_seal_sha256"`
	EvaluationArtifactSHA256 string                             `json:"evaluation_artifact_sha256"`
	Semantics                ResumeEpisodeSemantics             `json:"episode_semantics"`
	Interruptions            []OfflineResumeInterruptionReceipt `json:"interruptions"`
	LiveStaleProbe           *LiveStaleProbeReceipt             `json:"live_stale_probe,omitempty"`
	LiveStaleProbeSHA256     string                             `json:"live_stale_probe_sha256,omitempty"`
	GoalSuccess              bool                               `json:"goal_success"`
	ValidTerminalState       bool                               `json:"valid_terminal_state"`
	CausalAdmissionComplete  bool                               `json:"causal_admission_complete"`
	CleanDeskQualified       bool                               `json:"clean_desk_qualified"`
	Recovery                 RecoveryMetrics                    `json:"recovery"`
	InferenceStartedAt       time.Time                          `json:"inference_started_at"`
	InferenceExitedAt        time.Time                          `json:"inference_exited_at"`
	EvaluatorStartedAt       time.Time                          `json:"evaluator_started_at"`
	EvaluatorCompletedAt     time.Time                          `json:"evaluator_completed_at"`
}

type OfflineResumeGate struct {
	RequiredSchedules     int      `json:"required_schedules"`
	QualifiedSchedules    int      `json:"qualified_schedules"`
	RequiredInterruptions int      `json:"required_interruptions"`
	ProvenInterruptions   int      `json:"proven_interruptions"`
	StaleWriteClasses     int      `json:"stale_write_classes"`
	SemanticMismatches    int      `json:"semantic_mismatches"`
	RestorationMismatches int      `json:"restoration_mismatches"`
	ProjectionMismatches  int      `json:"projection_mismatches"`
	Reasons               []string `json:"reasons"`
	Passed                bool     `json:"passed"`
}

type OfflineResumeReceipt struct {
	Schema                  string                    `json:"schema"`
	PreregistrationSHA256   string                    `json:"preregistration_sha256"`
	BaselineArtifactSHA256  string                    `json:"baseline_artifact_sha256"`
	Runs                    []OfflineResumeRunReceipt `json:"runs"`
	Gate                    OfflineResumeGate         `json:"gate"`
	LastInferenceExitedAt   time.Time                 `json:"last_inference_exited_at"`
	FirstEvaluatorStartedAt time.Time                 `json:"first_evaluator_started_at"`
	CompletedAt             time.Time                 `json:"completed_at"`
	GateEvidenceQualified   bool                      `json:"gate_evidence_qualified"`
	PromotionEligible       bool                      `json:"promotion_eligible"`
}

func (receipt OfflineResumeReceipt) Validate(
	registration OfflineResumePreregistration,
	baseline ResumeBaselineArtifact,
) error {
	if err := registration.Validate(); err != nil {
		return err
	}
	if err := baseline.Validate(); err != nil {
		return err
	}
	registrationSHA, err := registration.SHA256()
	if err != nil {
		return err
	}
	if receipt.Schema != OfflineResumeReceiptSchemaV1 ||
		receipt.PreregistrationSHA256 != registrationSHA ||
		!validDigest(receipt.BaselineArtifactSHA256) || receipt.Runs == nil ||
		len(receipt.Runs) != len(registration.Schedules) ||
		receipt.LastInferenceExitedAt.IsZero() || receipt.FirstEvaluatorStartedAt.IsZero() ||
		receipt.FirstEvaluatorStartedAt.Before(receipt.LastInferenceExitedAt) ||
		receipt.CompletedAt.Before(receipt.FirstEvaluatorStartedAt) {
		return fmt.Errorf("offline Resume receipt authority is invalid")
	}
	lastInference, firstEvaluator, completedAt := time.Time{}, time.Time{}, time.Time{}
	for index := range receipt.Runs {
		if err := receipt.Runs[index].validate(registration.Schedules[index], baseline); err != nil {
			return fmt.Errorf("offline Resume run %d: %w", index+1, err)
		}
		resumeChronologyFromRun(
			receipt.Runs[index], &lastInference, &firstEvaluator, &completedAt,
		)
	}
	if receipt.LastInferenceExitedAt != lastInference ||
		receipt.FirstEvaluatorStartedAt != firstEvaluator || receipt.CompletedAt != completedAt {
		return fmt.Errorf("offline Resume aggregate chronology is not derived from sealed runs")
	}
	gate := deriveOfflineResumeGate(receipt.Runs, baseline)
	if !equalOfflineResumeGate(receipt.Gate, gate) ||
		receipt.GateEvidenceQualified != gate.Passed || receipt.PromotionEligible {
		return fmt.Errorf("offline Resume gate is not derived from sealed runs")
	}
	return nil
}

package cognitiongauntlet

import (
	"time"

	"github.com/gryph/omnidex/internal/cognition"
)

const OfflineScaleReceiptSchemaV1 = "omnidex.offline-scale-receipt.v1"

type OfflineScaleRunResult struct {
	Case                       OfflineScaleCase      `json:"case"`
	GeneratorVersion           string                `json:"generator_version"`
	Scenario                   cognition.ScenarioRef `json:"scenario"`
	OracleSHA256               string                `json:"oracle_sha256"`
	EpisodeSealSHA256          string                `json:"episode_seal_sha256"`
	RelevantSurfaceBytes       int64                 `json:"relevant_surface_bytes"`
	PeakContextBytes           int64                 `json:"peak_context_bytes"`
	ModelDecisions             int                   `json:"model_decisions"`
	RetrievalRounds            int                   `json:"retrieval_rounds"`
	GoalSuccess                bool                  `json:"goal_success"`
	ValidTerminalState         bool                  `json:"valid_terminal_state"`
	CausalAdmissionComplete    bool                  `json:"causal_admission_complete"`
	CleanDeskQualified         bool                  `json:"clean_desk_qualified"`
	CompetenceQualifiedSuccess bool                  `json:"competence_qualified_success"`
}

type OfflineScaleRunReceipt struct {
	Case                     OfflineScaleCase      `json:"case"`
	PromotionReceiptSHA256   string                `json:"promotion_receipt_sha256"`
	PublicAuthoritySHA256    string                `json:"public_authority_sha256"`
	EvaluationArtifactSHA256 string                `json:"evaluation_artifact_sha256"`
	ScaleEvidenceSHA256      string                `json:"scale_evidence_sha256"`
	Result                   OfflineScaleRunResult `json:"result"`
	InferenceStartedAt       time.Time             `json:"inference_started_at"`
	InferenceExitedAt        time.Time             `json:"inference_exited_at"`
	EvaluatorStartedAt       time.Time             `json:"evaluator_started_at"`
	EvaluatorCompletedAt     time.Time             `json:"evaluator_completed_at"`
}

type OfflineScaleReceipt struct {
	Schema                  string                   `json:"schema"`
	PreregistrationSHA256   string                   `json:"preregistration_sha256"`
	Authority               ScaleFamilyAuthority     `json:"authority"`
	Runs                    []OfflineScaleRunReceipt `json:"runs"`
	Report                  ScaleRailReport          `json:"report"`
	LastInferenceExitedAt   time.Time                `json:"last_inference_exited_at"`
	FirstEvaluatorStartedAt time.Time                `json:"first_evaluator_started_at"`
	CompletedAt             time.Time                `json:"completed_at"`
	GateEvidenceQualified   bool                     `json:"gate_evidence_qualified"`
	PromotionEligible       bool                     `json:"promotion_eligible"`
}

type VerifiedOfflineScaleReceipt struct {
	receipt OfflineScaleReceipt
}

func (verified VerifiedOfflineScaleReceipt) Receipt() OfflineScaleReceipt {
	copy := verified.receipt
	copy.Runs = append([]OfflineScaleRunReceipt{}, verified.receipt.Runs...)
	copy.Report.Measurements = append([]ScaleMeasurement{}, verified.receipt.Report.Measurements...)
	copy.Report.Gate.Reasons = append([]string{}, verified.receipt.Report.Gate.Reasons...)
	return copy
}

func (verified VerifiedOfflineScaleReceipt) PromotionEligible() bool {
	return verified.receipt.PromotionEligible
}

func (verified VerifiedOfflineScaleReceipt) GateEvidenceQualified() bool {
	return verified.receipt.GateEvidenceQualified
}

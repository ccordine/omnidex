package cognitiongauntlet

import (
	"fmt"
	"time"
)

const OfflineMatrixReceiptSchemaV2 = "omnidex.offline-cognition-matrix-receipt.v2"

type OfflineMatrixEvidenceClass string

const (
	MatrixIsolatedProcess    OfflineMatrixEvidenceClass = "isolated_process_model_evidence"
	MatrixBenchmarkOnly      OfflineMatrixEvidenceClass = "benchmark_only_model_evidence"
	MatrixOracleContaminated OfflineMatrixEvidenceClass = "oracle_packet_contaminated"
)

type OfflineMatrixRunReceipt struct {
	Case                          OfflineMatrixCase          `json:"case"`
	Variant                       Variant                    `json:"variant"`
	EvidenceClass                 OfflineMatrixEvidenceClass `json:"evidence_class"`
	PromotionReceiptSHA256        string                     `json:"promotion_receipt_sha256"`
	PublicRunAuthoritySHA256      string                     `json:"public_run_authority_sha256"`
	EpisodeSealSHA256             string                     `json:"episode_seal_sha256"`
	EvaluationArtifactSHA256      string                     `json:"evaluation_artifact_sha256"`
	OracleSHA256                  string                     `json:"oracle_sha256"`
	OracleQuality                 OracleQuality              `json:"oracle_quality"`
	OracleReferenceDecisionCost   int64                      `json:"oracle_reference_decision_cost"`
	TaskArchetype                 string                     `json:"task_archetype"`
	GoalSuccess                   bool                       `json:"goal_success"`
	ValidTerminalState            bool                       `json:"valid_terminal_state"`
	CausalAdmissionComplete       bool                       `json:"causal_admission_complete"`
	CleanDeskAvailable            bool                       `json:"clean_desk_available"`
	CleanDeskQualified            bool                       `json:"clean_desk_qualified"`
	CompetenceQualified           bool                       `json:"competence_qualified"`
	ModelCalls                    int                        `json:"model_calls"`
	ModelVisibleBytes             int64                      `json:"model_visible_bytes"`
	NativeInputTokens             int64                      `json:"native_input_tokens"`
	NativeOutputTokens            int64                      `json:"native_output_tokens"`
	ProviderTotalNanoseconds      int64                      `json:"provider_total_nanoseconds"`
	ProviderLoadNanoseconds       int64                      `json:"provider_load_nanoseconds"`
	ProviderPromptEvalNanoseconds int64                      `json:"provider_prompt_eval_nanoseconds"`
	ProviderEvalNanoseconds       int64                      `json:"provider_eval_nanoseconds"`
	PolicyWallMilliseconds        int64                      `json:"policy_wall_milliseconds"`
	NativeUsageComplete           bool                       `json:"native_usage_complete"`
	StationBudgetQualified        bool                       `json:"station_budget_qualified"`
	MissingCriticalRefs           int                        `json:"missing_critical_refs"`
	Reacquisitions                int                        `json:"reacquisitions"`
	ToolOperations                int                        `json:"tool_operations"`
	InferenceStartedAt            time.Time                  `json:"inference_started_at"`
	InferenceExitedAt             time.Time                  `json:"inference_exited_at"`
	EvaluatorStartedAt            time.Time                  `json:"evaluator_started_at"`
	EvaluatorCompletedAt          time.Time                  `json:"evaluator_completed_at"`
}

type OfflineMatrixGate struct {
	Policy                       CompetencePolicy `json:"policy"`
	BaselineVariant              Variant          `json:"baseline_variant"`
	CandidateVariant             Variant          `json:"candidate_variant"`
	Tasks                        int              `json:"tasks"`
	BaselineSuccesses            int              `json:"baseline_successes"`
	CandidateSuccesses           int              `json:"candidate_successes"`
	BaselineValidTerminals       int              `json:"baseline_valid_terminals"`
	CandidateValidTerminals      int              `json:"candidate_valid_terminals"`
	Rescues                      int              `json:"rescues"`
	Regressions                  int              `json:"regressions"`
	DiscordantPairs              int              `json:"discordant_pairs"`
	PairedPValueUpperPPM         int              `json:"paired_p_value_upper_ppm"`
	PairedLiftBasisPoints        int              `json:"paired_lift_basis_points"`
	SuccessLossBasisPoints       int              `json:"success_loss_basis_points"`
	MedianContextReductionPoints int              `json:"median_context_reduction_basis_points"`
	ReacquisitionDelta           int              `json:"reacquisition_delta"`
	ToolOperationDelta           int              `json:"tool_operation_delta"`
	Reasons                      []string         `json:"reasons"`
	Passed                       bool             `json:"passed"`
}

type OfflineMatrixReceipt struct {
	Schema                    string                     `json:"schema"`
	PreregistrationSHA256     string                     `json:"preregistration_sha256"`
	Runs                      []OfflineMatrixRunReceipt  `json:"runs"`
	DeterministicOracleBounds []OfflineMatrixOracleBound `json:"deterministic_oracle_bounds"`
	Tournament                OfflineMatrixTournament    `json:"tournament"`
	Gate                      OfflineMatrixGate          `json:"gate"`
	LastInferenceExitedAt     time.Time                  `json:"last_inference_exited_at"`
	FirstEvaluatorStartedAt   time.Time                  `json:"first_evaluator_started_at"`
	CompletedAt               time.Time                  `json:"completed_at"`
	GateEvidenceQualified     bool                       `json:"gate_evidence_qualified"`
	PromotionEligible         bool                       `json:"promotion_eligible"`
}

func (receipt OfflineMatrixReceipt) Validate(
	registration OfflineMatrixPreregistration,
) error {
	if err := registration.Validate(); err != nil {
		return err
	}
	registrationSHA, err := registration.SHA256()
	if err != nil {
		return err
	}
	if receipt.Schema != OfflineMatrixReceiptSchemaV2 ||
		receipt.PreregistrationSHA256 != registrationSHA ||
		receipt.Runs == nil || len(receipt.Runs) != registration.RunCount ||
		receipt.LastInferenceExitedAt.IsZero() || receipt.FirstEvaluatorStartedAt.IsZero() ||
		receipt.FirstEvaluatorStartedAt.Before(receipt.LastInferenceExitedAt) ||
		receipt.CompletedAt.Before(receipt.FirstEvaluatorStartedAt) {
		return fmt.Errorf("offline cognition matrix receipt authority is invalid")
	}
	expected := matrixCoordinates(registration)
	lastInference, firstEvaluator, completedAt := time.Time{}, time.Time{}, time.Time{}
	for index, run := range receipt.Runs {
		if err := run.validate(expected[index].Case, expected[index].Variant, registration); err != nil {
			return fmt.Errorf("offline matrix run %d: %w", index+1, err)
		}
		if run.InferenceExitedAt.After(lastInference) {
			lastInference = run.InferenceExitedAt
		}
		if firstEvaluator.IsZero() || run.EvaluatorStartedAt.Before(firstEvaluator) {
			firstEvaluator = run.EvaluatorStartedAt
		}
		if run.EvaluatorCompletedAt.After(completedAt) {
			completedAt = run.EvaluatorCompletedAt
		}
	}
	if receipt.LastInferenceExitedAt != lastInference ||
		receipt.FirstEvaluatorStartedAt != firstEvaluator || receipt.CompletedAt != completedAt {
		return fmt.Errorf("offline cognition matrix chronology is not derived from sealed runs")
	}
	bounds, err := deriveOfflineMatrixOracleBounds(registration, receipt.Runs)
	if err != nil || !equalOfflineMatrixOracleBounds(receipt.DeterministicOracleBounds, bounds) {
		return fmt.Errorf("offline cognition matrix oracle bounds are not evaluator-derived")
	}
	tournament, err := deriveOfflineMatrixTournament(registration, receipt.Runs)
	if err != nil || !equalOfflineMatrixTournament(receipt.Tournament, tournament) {
		return fmt.Errorf("offline cognition matrix tournament is not derived from its sealed runs")
	}
	derived, err := deriveOfflineMatrixGate(registration, receipt.Runs)
	if err != nil {
		return err
	}
	if !equalMatrixGate(receipt.Gate, derived) ||
		receipt.GateEvidenceQualified != derived.Passed || receipt.PromotionEligible {
		return fmt.Errorf("offline cognition matrix gate is not derived from its sealed runs")
	}
	return nil
}

func (run OfflineMatrixRunReceipt) validate(
	wantCase OfflineMatrixCase,
	wantVariant Variant,
	registration OfflineMatrixPreregistration,
) error {
	if run.Case != wantCase || run.Variant != wantVariant ||
		!validDigest(run.PromotionReceiptSHA256) ||
		!validDigest(run.PublicRunAuthoritySHA256) ||
		!validDigest(run.EpisodeSealSHA256) || !validDigest(run.EvaluationArtifactSHA256) ||
		!validDigest(run.OracleSHA256) || run.OracleReferenceDecisionCost <= 0 ||
		run.TaskArchetype != offlineScenarioTaskArchetype(run.Case.Suite) ||
		(run.OracleQuality != OracleOptimal && run.OracleQuality != OracleWitnessOnly) ||
		run.ModelCalls < 0 || run.ModelVisibleBytes < 0 || run.NativeInputTokens < 0 ||
		run.NativeOutputTokens < 0 || run.ProviderTotalNanoseconds < 0 ||
		run.ProviderLoadNanoseconds < 0 || run.ProviderPromptEvalNanoseconds < 0 ||
		run.ProviderEvalNanoseconds < 0 || run.PolicyWallMilliseconds < 0 ||
		run.MissingCriticalRefs < 0 || run.Reacquisitions < 0 ||
		run.ToolOperations < 0 || run.InferenceStartedAt.Before(registration.RegisteredAt) ||
		run.InferenceExitedAt.Before(run.InferenceStartedAt) ||
		run.EvaluatorStartedAt.Before(run.InferenceExitedAt) ||
		run.EvaluatorCompletedAt.Before(run.EvaluatorStartedAt) {
		return fmt.Errorf("run authority or measurement is invalid")
	}
	if run.ProviderTotalNanoseconds < run.ProviderLoadNanoseconds+
		run.ProviderPromptEvalNanoseconds+run.ProviderEvalNanoseconds ||
		run.CleanDeskQualified != (run.CleanDeskAvailable && run.MissingCriticalRefs == 0 &&
			run.NativeUsageComplete && run.StationBudgetQualified) ||
		run.CompetenceQualified != (run.GoalSuccess && run.ValidTerminalState &&
			run.CausalAdmissionComplete && run.CleanDeskQualified) {
		return fmt.Errorf("run competence qualification is not evaluator-derived")
	}
	wantClass := MatrixIsolatedProcess
	if wantVariant == VariantRawShell {
		wantClass = MatrixBenchmarkOnly
	} else if wantVariant == VariantOracleEvidence {
		wantClass = MatrixOracleContaminated
	}
	if run.EvidenceClass != wantClass {
		return fmt.Errorf("run evidence class is invalid")
	}
	return nil
}

type offlineMatrixCoordinate struct {
	Case    OfflineMatrixCase
	Variant Variant
}

func matrixCoordinates(registration OfflineMatrixPreregistration) []offlineMatrixCoordinate {
	coordinates := make([]offlineMatrixCoordinate, 0, registration.RunCount)
	for _, variant := range registration.Variants {
		for _, currentCase := range registration.Cases {
			coordinates = append(coordinates, offlineMatrixCoordinate{Case: currentCase, Variant: variant})
		}
	}
	return coordinates
}

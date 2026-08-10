package cognitiongauntlet

import (
	"fmt"
	"reflect"
	"time"
)

const OfflineTransferReceiptSchemaV1 = "omnidex.offline-transfer-receipt.v1"

type OfflineTransferRunReceipt struct {
	SurfaceVersion           string                `json:"surface_version"`
	PromotionReceiptSHA256   string                `json:"promotion_receipt_sha256"`
	PublicRunAuthoritySHA256 string                `json:"public_run_authority_sha256"`
	EvaluationArtifactSHA256 string                `json:"evaluation_artifact_sha256"`
	Result                   TransferEpisodeResult `json:"result"`
	InferenceStartedAt       time.Time             `json:"inference_started_at"`
	InferenceExitedAt        time.Time             `json:"inference_exited_at"`
	EvaluatorStartedAt       time.Time             `json:"evaluator_started_at"`
	EvaluatorCompletedAt     time.Time             `json:"evaluator_completed_at"`
}

type OfflineTransferReceipt struct {
	Schema                  string                      `json:"schema"`
	PreregistrationSHA256   string                      `json:"preregistration_sha256"`
	Authority               TransferAuthority           `json:"authority"`
	Runs                    []OfflineTransferRunReceipt `json:"runs"`
	Report                  TransferRailReport          `json:"report"`
	LastInferenceExitedAt   time.Time                   `json:"last_inference_exited_at"`
	FirstEvaluatorStartedAt time.Time                   `json:"first_evaluator_started_at"`
	CompletedAt             time.Time                   `json:"completed_at"`
	GateEvidenceQualified   bool                        `json:"gate_evidence_qualified"`
	PromotionEligible       bool                        `json:"promotion_eligible"`
}

type VerifiedOfflineTransferReceipt struct {
	receipt OfflineTransferReceipt
}

func (verified VerifiedOfflineTransferReceipt) Receipt() OfflineTransferReceipt {
	copy := verified.receipt
	copy.Authority.SurfaceVersions = append([]string{}, verified.receipt.Authority.SurfaceVersions...)
	copy.Runs = append([]OfflineTransferRunReceipt{}, verified.receipt.Runs...)
	for index := range copy.Runs {
		copy.Runs[index].Result.CausalAcquisition.AcquisitionTraceRefs = append(
			[]string{}, verified.receipt.Runs[index].Result.CausalAcquisition.AcquisitionTraceRefs...,
		)
	}
	copy.Report.Authority.SurfaceVersions = append(
		[]string{}, verified.receipt.Report.Authority.SurfaceVersions...,
	)
	copy.Report.Episodes = append([]TransferEpisodeResult{}, verified.receipt.Report.Episodes...)
	for index := range copy.Report.Episodes {
		copy.Report.Episodes[index].CausalAcquisition.AcquisitionTraceRefs = append(
			[]string{}, verified.receipt.Report.Episodes[index].CausalAcquisition.AcquisitionTraceRefs...,
		)
	}
	copy.Report.Gate.Reasons = append([]string{}, verified.receipt.Report.Gate.Reasons...)
	return copy
}

func (verified VerifiedOfflineTransferReceipt) PromotionEligible() bool {
	return verified.receipt.PromotionEligible
}

func (verified VerifiedOfflineTransferReceipt) GateEvidenceQualified() bool {
	return verified.receipt.GateEvidenceQualified
}

func (receipt OfflineTransferReceipt) Validate(
	registration OfflineTransferPreregistration,
) error {
	if err := registration.Validate(); err != nil {
		return err
	}
	registrationSHA, err := registration.SHA256()
	if err != nil {
		return err
	}
	if receipt.Schema != OfflineTransferReceiptSchemaV1 ||
		receipt.PreregistrationSHA256 != registrationSHA ||
		len(receipt.Runs) != registration.RunCount ||
		receipt.LastInferenceExitedAt.IsZero() || receipt.FirstEvaluatorStartedAt.IsZero() ||
		receipt.FirstEvaluatorStartedAt.Before(receipt.LastInferenceExitedAt) ||
		receipt.CompletedAt.Before(receipt.FirstEvaluatorStartedAt) {
		return fmt.Errorf("offline Transfer receipt authority is invalid")
	}
	if err := validateOfflineTransferAuthority(receipt.Authority, registration); err != nil {
		return err
	}
	authoritySHA, err := digestJSON(receipt.Authority)
	if err != nil {
		return err
	}
	episodes := make([]TransferEpisodeResult, len(receipt.Runs))
	lastInference, firstEvaluator, completedAt := time.Time{}, time.Time{}, time.Time{}
	for index, run := range receipt.Runs {
		version, _ := registration.Plan.Surfaces[index].Version()
		if err := run.validate(version, authoritySHA, registration.RegisteredAt); err != nil {
			return fmt.Errorf("offline Transfer run %d: %w", index+1, err)
		}
		episodes[index] = run.Result
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
	if lastInference != receipt.LastInferenceExitedAt ||
		firstEvaluator != receipt.FirstEvaluatorStartedAt || completedAt != receipt.CompletedAt {
		return fmt.Errorf("offline Transfer aggregate chronology was not derived from sealed runs")
	}
	report, err := EvaluateTransferRail(receipt.Authority, episodes)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(receipt.Report, report) ||
		receipt.GateEvidenceQualified != report.Gate.Passed || receipt.PromotionEligible {
		return fmt.Errorf("offline Transfer report is not derived from sealed surface episodes")
	}
	return nil
}

func (run OfflineTransferRunReceipt) validate(
	wantSurface string,
	authoritySHA string,
	registeredAt time.Time,
) error {
	if run.SurfaceVersion != wantSurface || run.Result.SurfaceVersion != wantSurface ||
		run.Result.AuthoritySHA256 != authoritySHA ||
		!validDigest(run.PromotionReceiptSHA256) ||
		!validDigest(run.PublicRunAuthoritySHA256) ||
		!validDigest(run.EvaluationArtifactSHA256) ||
		run.InferenceStartedAt.Before(registeredAt) ||
		run.InferenceExitedAt.Before(run.InferenceStartedAt) ||
		run.EvaluatorStartedAt.Before(run.InferenceExitedAt) ||
		run.EvaluatorCompletedAt.Before(run.EvaluatorStartedAt) {
		return fmt.Errorf("offline Transfer run authority is invalid")
	}
	return run.Result.Validate()
}

func validateOfflineTransferAuthority(
	authority TransferAuthority,
	registration OfflineTransferPreregistration,
) error {
	if err := authority.Validate(); err != nil {
		return err
	}
	versions, err := sortedSurfaceVersions(registration.Plan.Surfaces)
	if err != nil {
		return err
	}
	if authority.CaseID != registration.Workload.CaseID() ||
		authority.TaskSuite != registration.Plan.Suite ||
		authority.Seed != registration.Plan.Seed || authority.Repetition != registration.Plan.Repetition ||
		authority.Variant != VariantFullCognition ||
		authority.RatGeneration != registration.Fixed.RatGeneration ||
		authority.Budget != registration.Fixed.Budget ||
		authority.Runtime != registration.Fixed.RuntimeFingerprint ||
		!reflect.DeepEqual(authority.SurfaceVersions, versions) {
		return fmt.Errorf("offline Transfer authority changed its preregistered task or runtime")
	}
	return nil
}

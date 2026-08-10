package cognitiongauntlet

import (
	"context"
	"fmt"
	"time"
)

func RunOfflineTransfer(
	ctx context.Context,
	config OfflineTransferConfig,
	executable string,
) (VerifiedOfflineTransferReceipt, error) {
	if ctx == nil {
		return VerifiedOfflineTransferReceipt{}, fmt.Errorf("offline Transfer context is nil")
	}
	if err := config.ValidateStart(); err != nil {
		return VerifiedOfflineTransferReceipt{}, err
	}
	registration, err := LoadOfflineTransferPreregistration(config.Paths().Preregistration)
	if err != nil {
		return VerifiedOfflineTransferReceipt{}, err
	}
	inferences := make([]offlinePromotionInference, len(registration.Plan.Surfaces))
	lastInferenceExitedAt := time.Time{}
	for index, surface := range registration.Plan.Surfaces {
		runConfig, err := config.runConfig(registration, surface)
		if err != nil {
			return VerifiedOfflineTransferReceipt{}, fmt.Errorf("prepare Transfer surface %d: %w", index+1, err)
		}
		inference, err := runOfflinePromotionInference(ctx, runConfig, executable)
		if err != nil {
			return VerifiedOfflineTransferReceipt{}, fmt.Errorf("execute Transfer inference %d: %w", index+1, err)
		}
		if inference.inferenceStartedAt.Before(registration.RegisteredAt) {
			return VerifiedOfflineTransferReceipt{}, fmt.Errorf("Transfer inference started before preregistration")
		}
		inferences[index] = inference
		if inference.inferenceExitedAt.After(lastInferenceExitedAt) {
			lastInferenceExitedAt = inference.inferenceExitedAt
		}
	}
	if err := requireAllTransferEvaluationsAbsent(inferences); err != nil {
		return VerifiedOfflineTransferReceipt{}, err
	}
	firstEvaluatorStartedAt := time.Time{}
	for index := range inferences {
		promotion, err := inferences[index].evaluate(ctx)
		if err != nil {
			return VerifiedOfflineTransferReceipt{}, fmt.Errorf("evaluate Transfer surface %d: %w", index+1, err)
		}
		if promotion.EvaluatorStartedAt.Before(lastInferenceExitedAt) {
			return VerifiedOfflineTransferReceipt{}, fmt.Errorf("Transfer evaluator started before every inference exited")
		}
		if firstEvaluatorStartedAt.IsZero() || promotion.EvaluatorStartedAt.Before(firstEvaluatorStartedAt) {
			firstEvaluatorStartedAt = promotion.EvaluatorStartedAt
		}
	}
	artifacts, err := loadAllOfflineTransferArtifacts(config, registration)
	if err != nil {
		return VerifiedOfflineTransferReceipt{}, err
	}
	receipt, err := buildOfflineTransferReceipt(registration, artifacts)
	if err != nil {
		return VerifiedOfflineTransferReceipt{}, err
	}
	if err := SealOfflineTransferReceipt(config.Paths().Receipt, receipt, registration); err != nil {
		return VerifiedOfflineTransferReceipt{}, err
	}
	return LoadVerifiedOfflineTransferReceipt(config)
}

func requireAllTransferEvaluationsAbsent(inferences []offlinePromotionInference) error {
	if len(inferences) < 2 {
		return fmt.Errorf("offline Transfer has fewer than two inference runs")
	}
	for index, inference := range inferences {
		if err := ensureAbsent(inference.paths.Evaluation, "Transfer private evaluation"); err != nil {
			return fmt.Errorf("Transfer surface %d: %w", index+1, err)
		}
		if err := ensureAbsent(inference.paths.Receipt, "Transfer promotion receipt"); err != nil {
			return fmt.Errorf("Transfer surface %d: %w", index+1, err)
		}
	}
	return nil
}

func loadAllOfflineTransferArtifacts(
	config OfflineTransferConfig,
	registration OfflineTransferPreregistration,
) ([]offlineTransferArtifacts, error) {
	artifacts := make([]offlineTransferArtifacts, len(registration.Plan.Surfaces))
	for index, surface := range registration.Plan.Surfaces {
		artifact, err := loadOfflineTransferArtifacts(config, registration, surface)
		if err != nil {
			return nil, fmt.Errorf("load Transfer surface %d: %w", index+1, err)
		}
		artifacts[index] = artifact
	}
	return artifacts, nil
}

func buildOfflineTransferReceipt(
	registration OfflineTransferPreregistration,
	artifacts []offlineTransferArtifacts,
) (OfflineTransferReceipt, error) {
	authority, err := deriveOfflineTransferAuthority(registration, artifacts)
	if err != nil {
		return OfflineTransferReceipt{}, err
	}
	runs := make([]OfflineTransferRunReceipt, len(artifacts))
	episodes := make([]TransferEpisodeResult, len(artifacts))
	for index, artifact := range artifacts {
		run, err := buildOfflineTransferRunReceipt(authority, registration, artifact)
		if err != nil {
			return OfflineTransferReceipt{}, fmt.Errorf("bind Transfer surface %d: %w", index+1, err)
		}
		runs[index], episodes[index] = run, run.Result
	}
	lastInferenceExitedAt, firstEvaluatorStartedAt, completedAt, err :=
		transferAggregateChronology(runs)
	if err != nil {
		return OfflineTransferReceipt{}, err
	}
	report, err := EvaluateTransferRail(authority, episodes)
	if err != nil {
		return OfflineTransferReceipt{}, err
	}
	registrationSHA, err := registration.SHA256()
	if err != nil {
		return OfflineTransferReceipt{}, err
	}
	receipt := OfflineTransferReceipt{
		Schema:                OfflineTransferReceiptSchemaV1,
		PreregistrationSHA256: registrationSHA, Authority: authority,
		Runs: runs, Report: report, LastInferenceExitedAt: lastInferenceExitedAt,
		FirstEvaluatorStartedAt: firstEvaluatorStartedAt, CompletedAt: completedAt,
		GateEvidenceQualified: report.Gate.Passed, PromotionEligible: false,
	}
	return receipt, receipt.Validate(registration)
}

func transferAggregateChronology(
	runs []OfflineTransferRunReceipt,
) (time.Time, time.Time, time.Time, error) {
	lastInference, firstEvaluator, completedAt := time.Time{}, time.Time{}, time.Time{}
	for _, run := range runs {
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
	if lastInference.IsZero() || firstEvaluator.IsZero() || completedAt.IsZero() {
		return time.Time{}, time.Time{}, time.Time{}, fmt.Errorf("Transfer chronology is incomplete")
	}
	return lastInference, firstEvaluator, completedAt, nil
}

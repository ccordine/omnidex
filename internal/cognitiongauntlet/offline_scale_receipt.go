package cognitiongauntlet

import (
	"fmt"
	"reflect"
	"time"
)

func buildOfflineScaleReceipt(
	registration OfflineScalePreregistration,
	artifacts []offlineScaleArtifacts,
	lastInferenceExitedAt time.Time,
	firstEvaluatorStartedAt time.Time,
) (OfflineScaleReceipt, error) {
	authority, err := deriveOfflineScaleAuthority(registration, artifacts)
	if err != nil {
		return OfflineScaleReceipt{}, err
	}
	runs := make([]OfflineScaleRunReceipt, len(artifacts))
	for index, artifact := range artifacts {
		run, err := buildOfflineScaleRunReceipt(registration, authority, artifact)
		if err != nil {
			return OfflineScaleReceipt{}, fmt.Errorf("bind Scale run %d: %w", index+1, err)
		}
		runs[index] = run
	}
	measurements, err := deriveOfflineScaleMeasurements(registration, authority, runs)
	if err != nil {
		return OfflineScaleReceipt{}, err
	}
	report, err := EvaluateScaleRail(authority, measurements)
	if err != nil {
		return OfflineScaleReceipt{}, err
	}
	registrationSHA, err := registration.SHA256()
	if err != nil {
		return OfflineScaleReceipt{}, err
	}
	derivedLast, derivedFirst, completedAt, err := scaleAggregateChronology(runs)
	if err != nil {
		return OfflineScaleReceipt{}, err
	}
	if derivedLast != lastInferenceExitedAt || derivedFirst != firstEvaluatorStartedAt {
		return OfflineScaleReceipt{}, fmt.Errorf("Scale aggregate chronology diverged from sealed runs")
	}
	receipt := OfflineScaleReceipt{
		Schema: OfflineScaleReceiptSchemaV1, PreregistrationSHA256: registrationSHA,
		Authority: authority, Runs: runs, Report: report,
		LastInferenceExitedAt:   lastInferenceExitedAt,
		FirstEvaluatorStartedAt: firstEvaluatorStartedAt, CompletedAt: completedAt,
		GateEvidenceQualified: report.Gate.Passed, PromotionEligible: false,
	}
	return receipt, receipt.Validate(registration)
}

func (receipt OfflineScaleReceipt) Validate(
	registration OfflineScalePreregistration,
) error {
	if err := registration.Validate(); err != nil {
		return err
	}
	registrationSHA, err := registration.SHA256()
	if err != nil {
		return err
	}
	if receipt.Schema != OfflineScaleReceiptSchemaV1 ||
		receipt.PreregistrationSHA256 != registrationSHA ||
		len(receipt.Runs) != registration.RunCount ||
		receipt.LastInferenceExitedAt.IsZero() || receipt.FirstEvaluatorStartedAt.IsZero() ||
		receipt.FirstEvaluatorStartedAt.Before(receipt.LastInferenceExitedAt) ||
		receipt.CompletedAt.Before(receipt.FirstEvaluatorStartedAt) {
		return fmt.Errorf("offline Scale receipt authority is invalid")
	}
	if err := validateOfflineScaleAuthority(receipt.Authority, registration); err != nil {
		return err
	}
	lastInference, firstEvaluator, completedAt := time.Time{}, time.Time{}, time.Time{}
	for index, run := range receipt.Runs {
		if err := run.validate(
			registration.Cases[index], registration.RegisteredAt, receipt.Authority.Budget,
		); err != nil {
			return fmt.Errorf("offline Scale run %d: %w", index+1, err)
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
	if lastInference != receipt.LastInferenceExitedAt ||
		firstEvaluator != receipt.FirstEvaluatorStartedAt || completedAt != receipt.CompletedAt {
		return fmt.Errorf("offline Scale receipt chronology was caller-authored")
	}
	measurements, err := deriveOfflineScaleMeasurements(
		registration, receipt.Authority, receipt.Runs,
	)
	if err != nil {
		return err
	}
	report, err := EvaluateScaleRail(receipt.Authority, measurements)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(receipt.Report, report) ||
		receipt.GateEvidenceQualified != report.Gate.Passed || receipt.PromotionEligible {
		return fmt.Errorf("offline Scale report is not derived from sealed runs")
	}
	return nil
}

func scaleAggregateChronology(
	runs []OfflineScaleRunReceipt,
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
		return time.Time{}, time.Time{}, time.Time{}, fmt.Errorf("Scale chronology is incomplete")
	}
	return lastInference, firstEvaluator, completedAt, nil
}

func equalOfflineScaleReceipt(left, right OfflineScaleReceipt) bool {
	return reflect.DeepEqual(left, right)
}

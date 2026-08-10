package cognitiongauntlet

import (
	"context"
	"fmt"
	"reflect"
	"time"
)

func RunOfflineMatrix(
	ctx context.Context,
	config OfflineMatrixConfig,
	executable string,
) (VerifiedOfflineMatrixReceipt, error) {
	if ctx == nil {
		return VerifiedOfflineMatrixReceipt{}, fmt.Errorf("offline cognition matrix context is nil")
	}
	if err := config.ValidateStart(); err != nil {
		return VerifiedOfflineMatrixReceipt{}, err
	}
	registration, err := LoadOfflineMatrixPreregistration(config.Paths().Preregistration)
	if err != nil {
		return VerifiedOfflineMatrixReceipt{}, err
	}
	coordinates := matrixCoordinates(registration)
	inferences := make([]offlinePromotionInference, len(coordinates))
	lastInferenceExitedAt := time.Time{}
	for index, coordinate := range coordinates {
		runConfig, err := config.runConfig(coordinate.Case, coordinate.Variant)
		if err != nil {
			return VerifiedOfflineMatrixReceipt{}, fmt.Errorf("prepare matrix run %d: %w", index+1, err)
		}
		inference, err := runOfflinePromotionInference(ctx, runConfig, executable)
		if err != nil {
			return VerifiedOfflineMatrixReceipt{}, fmt.Errorf("execute matrix inference %d: %w", index+1, err)
		}
		if inference.inferenceStartedAt.Before(registration.RegisteredAt) {
			return VerifiedOfflineMatrixReceipt{}, fmt.Errorf("matrix inference started before preregistration")
		}
		inferences[index] = inference
		if inference.inferenceExitedAt.After(lastInferenceExitedAt) {
			lastInferenceExitedAt = inference.inferenceExitedAt
		}
	}
	if err := requireAllMatrixEvaluationsAbsent(inferences); err != nil {
		return VerifiedOfflineMatrixReceipt{}, err
	}
	runs := make([]OfflineMatrixRunReceipt, len(coordinates))
	firstEvaluatorStartedAt := time.Time{}
	for index, inference := range inferences {
		promotion, err := inference.evaluate(ctx)
		if err != nil {
			return VerifiedOfflineMatrixReceipt{}, fmt.Errorf("evaluate matrix run %d: %w", index+1, err)
		}
		if promotion.EvaluatorStartedAt.Before(lastInferenceExitedAt) {
			return VerifiedOfflineMatrixReceipt{}, fmt.Errorf("matrix evaluator started before every inference exited")
		}
		if firstEvaluatorStartedAt.IsZero() || promotion.EvaluatorStartedAt.Before(firstEvaluatorStartedAt) {
			firstEvaluatorStartedAt = promotion.EvaluatorStartedAt
		}
		loaded, promotionSHA, err := loadOfflinePromotionReceiptArtifact(inference.paths.Receipt)
		if err != nil {
			return VerifiedOfflineMatrixReceipt{}, err
		}
		if !reflect.DeepEqual(loaded, promotion) {
			return VerifiedOfflineMatrixReceipt{}, fmt.Errorf("matrix promotion receipt changed after sealing")
		}
		evaluation, err := promotion.VerifyEvaluationArtifact(inference.paths.Evaluation)
		if err != nil {
			return VerifiedOfflineMatrixReceipt{}, err
		}
		runs[index], err = buildOfflineMatrixRunReceipt(
			coordinates[index].Case, coordinates[index].Variant,
			promotion, promotionSHA, inference.episode, evaluation,
		)
		if err != nil {
			return VerifiedOfflineMatrixReceipt{}, err
		}
	}
	gate, err := deriveOfflineMatrixGate(registration, runs)
	if err != nil {
		return VerifiedOfflineMatrixReceipt{}, err
	}
	tournament, err := deriveOfflineMatrixTournament(registration, runs)
	if err != nil {
		return VerifiedOfflineMatrixReceipt{}, err
	}
	oracleBounds, err := deriveOfflineMatrixOracleBounds(registration, runs)
	if err != nil {
		return VerifiedOfflineMatrixReceipt{}, err
	}
	derivedLast, derivedFirst, derivedCompleted, err := matrixAggregateChronology(runs)
	if err != nil {
		return VerifiedOfflineMatrixReceipt{}, err
	}
	if derivedLast != lastInferenceExitedAt || derivedFirst != firstEvaluatorStartedAt {
		return VerifiedOfflineMatrixReceipt{}, fmt.Errorf("matrix aggregate chronology diverged from sealed runs")
	}
	releaseCoverage, releaseCoverageSHA, err := deriveReleaseMatrixCoverage(registration)
	if err != nil {
		return VerifiedOfflineMatrixReceipt{}, err
	}
	receipt := OfflineMatrixReceipt{
		Schema:                OfflineMatrixReceiptSchemaV3,
		PreregistrationSHA256: config.PreregistrationSHA256,
		Runs:                  runs, DeterministicOracleBounds: oracleBounds, Tournament: tournament,
		Gate: gate, LastInferenceExitedAt: lastInferenceExitedAt,
		FirstEvaluatorStartedAt:  firstEvaluatorStartedAt,
		CompletedAt:              derivedCompleted,
		GateEvidenceQualified:    gate.Passed,
		ReleaseCoverageQualified: releaseCoverage,
		ReleaseCoverageSHA256:    releaseCoverageSHA,
		PromotionEligible:        false,
	}
	if err := receipt.Validate(registration); err != nil {
		return VerifiedOfflineMatrixReceipt{}, err
	}
	if err := VerifyOfflineMatrixReceipt(config, receipt); err != nil {
		return VerifiedOfflineMatrixReceipt{}, err
	}
	if err := SealOfflineMatrixReceipt(config.Paths().Receipt, receipt, registration); err != nil {
		return VerifiedOfflineMatrixReceipt{}, err
	}
	return LoadVerifiedOfflineMatrixReceipt(config)
}

func matrixAggregateChronology(
	runs []OfflineMatrixRunReceipt,
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
		return time.Time{}, time.Time{}, time.Time{}, fmt.Errorf("matrix chronology is incomplete")
	}
	return lastInference, firstEvaluator, completedAt, nil
}

func requireAllMatrixEvaluationsAbsent(inferences []offlinePromotionInference) error {
	if len(inferences) == 0 {
		return fmt.Errorf("offline cognition matrix has no inference runs")
	}
	for index, inference := range inferences {
		if err := ensureAbsent(inference.paths.Evaluation, "matrix private evaluation"); err != nil {
			return fmt.Errorf("matrix run %d: %w", index+1, err)
		}
		if err := ensureAbsent(inference.paths.Receipt, "matrix promotion receipt"); err != nil {
			return fmt.Errorf("matrix run %d: %w", index+1, err)
		}
	}
	return nil
}

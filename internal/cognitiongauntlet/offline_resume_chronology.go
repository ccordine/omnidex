package cognitiongauntlet

import (
	"fmt"
	"time"
)

func resumeAggregateChronology(
	runs []OfflineResumeRunReceipt,
) (time.Time, time.Time, time.Time, error) {
	lastInference, firstEvaluator, completedAt := time.Time{}, time.Time{}, time.Time{}
	for _, run := range runs {
		resumeChronologyFromRun(run, &lastInference, &firstEvaluator, &completedAt)
	}
	if lastInference.IsZero() || firstEvaluator.IsZero() || completedAt.IsZero() {
		return time.Time{}, time.Time{}, time.Time{}, fmt.Errorf("Resume chronology is incomplete")
	}
	return lastInference, firstEvaluator, completedAt, nil
}

func resumeChronologyFromRun(
	run OfflineResumeRunReceipt,
	lastInference *time.Time,
	firstEvaluator *time.Time,
	completedAt *time.Time,
) {
	includeResumeChronology(
		run.InferenceExitedAt, run.EvaluatorStartedAt, run.EvaluatorCompletedAt,
		lastInference, firstEvaluator, completedAt,
	)
	if run.LiveStaleProbe == nil {
		return
	}
	for _, proof := range run.LiveStaleProbe.Probes {
		includeResumeChronology(
			proof.InferenceExitedAt, proof.EvaluatorStartedAt, proof.EvaluatorCompletedAt,
			lastInference, firstEvaluator, completedAt,
		)
	}
}

func includeResumeChronology(
	inferenceExitedAt time.Time,
	evaluatorStartedAt time.Time,
	evaluatorCompletedAt time.Time,
	lastInference *time.Time,
	firstEvaluator *time.Time,
	completedAt *time.Time,
) {
	if inferenceExitedAt.After(*lastInference) {
		*lastInference = inferenceExitedAt
	}
	if firstEvaluator.IsZero() || evaluatorStartedAt.Before(*firstEvaluator) {
		*firstEvaluator = evaluatorStartedAt
	}
	if evaluatorCompletedAt.After(*completedAt) {
		*completedAt = evaluatorCompletedAt
	}
}

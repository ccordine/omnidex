package cognitiongauntlet

import (
	"context"
	"fmt"
	"reflect"
	"time"
)

func RunOfflineResume(
	ctx context.Context,
	config OfflineResumeConfig,
	executable string,
) (VerifiedOfflineResumeReceipt, error) {
	if ctx == nil {
		return VerifiedOfflineResumeReceipt{}, fmt.Errorf("offline Resume context is nil")
	}
	if err := config.ValidateStart(); err != nil {
		return VerifiedOfflineResumeReceipt{}, err
	}
	registration, err := LoadOfflineResumePreregistration(config.Paths().Preregistration)
	if err != nil {
		return VerifiedOfflineResumeReceipt{}, err
	}
	inferences := make([]offlineResumeInference, len(registration.Schedules))
	baselineConfig, err := config.runConfig(registration, registration.Schedules[0])
	if err != nil {
		return VerifiedOfflineResumeReceipt{}, err
	}
	baselineInference, baseline, baselineSHA, err := runOfflineResumeUninterruptedInference(
		ctx, baselineConfig, registration.Schedules[0], executable,
	)
	if err != nil {
		return VerifiedOfflineResumeReceipt{}, err
	}
	inferences[0] = baselineInference
	lastInferenceExitedAt := baselineInference.promotion.inferenceExitedAt
	for index := 1; index < len(registration.Schedules); index++ {
		schedule := registration.Schedules[index]
		runConfig, err := config.runConfig(registration, schedule)
		if err != nil {
			return VerifiedOfflineResumeReceipt{}, err
		}
		var inference offlineResumeInference
		switch schedule.Kind {
		case ResumeOneSeededKill, ResumeFiveSeededKills:
			inference, err = runOfflineResumeFixedInference(ctx, runConfig, schedule, executable)
		case ResumeEveryDecision:
			inference, err = runOfflineResumeEveryDecisionInference(ctx, runConfig, schedule, executable)
		case ResumeLiveInferenceExpiry:
			inference, err = runOfflineResumeLiveInference(ctx, runConfig, schedule, executable, baseline)
		default:
			err = fmt.Errorf("offline Resume schedule %q has no executor", schedule.Kind)
		}
		if err != nil {
			return VerifiedOfflineResumeReceipt{}, fmt.Errorf("execute Resume inference %d: %w", index+1, err)
		}
		if schedule.Kind != ResumeLiveInferenceExpiry {
			inference, err = sealOfflineResumeInferenceEvidence(inference, baseline, nil, "")
			if err != nil {
				return VerifiedOfflineResumeReceipt{}, err
			}
		}
		inferences[index] = inference
		if exitedAt := latestResumeInferenceExit(inference); exitedAt.After(lastInferenceExitedAt) {
			lastInferenceExitedAt = exitedAt
		}
	}
	if err := requireAllResumeEvaluationsAbsent(inferences); err != nil {
		return VerifiedOfflineResumeReceipt{}, err
	}
	runs := make([]OfflineResumeRunReceipt, len(inferences))
	firstEvaluatorStartedAt := time.Time{}
	for index, inference := range inferences {
		var promotion OfflinePromotionReceipt
		var evaluation Evaluation
		var promotionSHA string
		if inference.schedule.Kind == ResumeLiveInferenceExpiry {
			inference, promotion, promotionSHA, evaluation, err = evaluateLiveResumeProbes(
				ctx, inference, baseline, lastInferenceExitedAt,
			)
			if err == nil {
				inferences[index] = inference
			}
		} else {
			promotion, err = inference.promotion.evaluate(ctx)
			if err == nil {
				var loaded OfflinePromotionReceipt
				loaded, promotionSHA, err = loadOfflinePromotionReceiptArtifact(
					inference.promotion.paths.Receipt,
				)
				if err == nil && !reflect.DeepEqual(loaded, promotion) {
					err = fmt.Errorf("Resume promotion receipt changed after sealing")
				}
				if err == nil {
					evaluation, err = promotion.VerifyEvaluationArtifact(
						inference.promotion.paths.Evaluation,
					)
				}
			}
		}
		if err != nil {
			return VerifiedOfflineResumeReceipt{}, fmt.Errorf("evaluate Resume run %d: %w", index+1, err)
		}
		if promotion.EvaluatorStartedAt.Before(lastInferenceExitedAt) {
			return VerifiedOfflineResumeReceipt{}, fmt.Errorf("Resume evaluator started before every inference exited")
		}
		if firstEvaluatorStartedAt.IsZero() || promotion.EvaluatorStartedAt.Before(firstEvaluatorStartedAt) {
			firstEvaluatorStartedAt = promotion.EvaluatorStartedAt
		}
		runs[index], err = buildOfflineResumeRunReceipt(
			inference, promotion, promotionSHA, evaluation, baseline,
		)
		if err != nil {
			return VerifiedOfflineResumeReceipt{}, err
		}
	}
	derivedLast, derivedFirst, derivedCompleted, err := resumeAggregateChronology(runs)
	if err != nil {
		return VerifiedOfflineResumeReceipt{}, err
	}
	if derivedLast != lastInferenceExitedAt || derivedFirst != firstEvaluatorStartedAt {
		return VerifiedOfflineResumeReceipt{}, fmt.Errorf("Resume aggregate chronology diverged from sealed runs")
	}
	receipt := OfflineResumeReceipt{
		Schema:                 OfflineResumeReceiptSchemaV1,
		PreregistrationSHA256:  config.PreregistrationSHA256,
		BaselineArtifactSHA256: baselineSHA, Runs: runs,
		LastInferenceExitedAt:   lastInferenceExitedAt,
		FirstEvaluatorStartedAt: firstEvaluatorStartedAt, CompletedAt: derivedCompleted,
	}
	receipt.Gate = deriveOfflineResumeGate(receipt.Runs, baseline)
	receipt.GateEvidenceQualified = receipt.Gate.Passed
	receipt.PromotionEligible = false
	if err := receipt.Validate(registration, baseline); err != nil {
		return VerifiedOfflineResumeReceipt{}, err
	}
	if err := VerifyOfflineResumeReceipt(config, receipt); err != nil {
		return VerifiedOfflineResumeReceipt{}, err
	}
	if err := SealOfflineResumeReceipt(
		config.Paths().Receipt, receipt, registration, baseline,
	); err != nil {
		return VerifiedOfflineResumeReceipt{}, err
	}
	return LoadVerifiedOfflineResumeReceipt(config)
}

func requireAllResumeEvaluationsAbsent(inferences []offlineResumeInference) error {
	if len(inferences) != 5 {
		return fmt.Errorf("Resume requires all five preregistered inference schedules")
	}
	for index, inference := range inferences {
		if err := requireInferencePrivateOutputsAbsent(inference.promotion.paths); err != nil {
			return fmt.Errorf("Resume inference %d: %w", index+1, err)
		}
		for probeIndex, probe := range inference.liveProbeRuns {
			if err := requireInferencePrivateOutputsAbsent(probe.promotion.paths); err != nil {
				return fmt.Errorf(
					"Resume inference %d live probe %d: %w", index+1, probeIndex+1, err,
				)
			}
		}
	}
	return nil
}

func latestResumeInferenceExit(inference offlineResumeInference) time.Time {
	latest := inference.promotion.inferenceExitedAt
	for _, probe := range inference.liveProbeRuns {
		if probe.promotion.inferenceExitedAt.After(latest) {
			latest = probe.promotion.inferenceExitedAt
		}
	}
	return latest
}

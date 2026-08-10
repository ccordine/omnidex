package cognitiongauntlet

import (
	"context"
	"fmt"
	"time"
)

func RunOfflineScale(
	ctx context.Context,
	config OfflineScaleConfig,
	executable string,
) (VerifiedOfflineScaleReceipt, error) {
	if ctx == nil {
		return VerifiedOfflineScaleReceipt{}, fmt.Errorf("offline Scale context is nil")
	}
	if err := config.ValidateStart(); err != nil {
		return VerifiedOfflineScaleReceipt{}, err
	}
	registration, err := LoadOfflineScalePreregistration(config.Paths().Preregistration)
	if err != nil {
		return VerifiedOfflineScaleReceipt{}, err
	}
	inferences, lastInferenceExitedAt, err := runOfflineScaleInferences(
		ctx, config, registration, executable,
	)
	if err != nil {
		return VerifiedOfflineScaleReceipt{}, err
	}
	firstEvaluatorStartedAt := time.Time{}
	for index, inference := range inferences {
		receipt, err := evaluateOfflineScaleInference(ctx, config, inference)
		if err != nil {
			return VerifiedOfflineScaleReceipt{}, fmt.Errorf("evaluate Scale run %d: %w", index+1, err)
		}
		if receipt.EvaluatorStartedAt.Before(lastInferenceExitedAt) {
			return VerifiedOfflineScaleReceipt{}, fmt.Errorf("Scale evaluator started before all inference exited")
		}
		if firstEvaluatorStartedAt.IsZero() || receipt.EvaluatorStartedAt.Before(firstEvaluatorStartedAt) {
			firstEvaluatorStartedAt = receipt.EvaluatorStartedAt
		}
	}
	artifacts, err := loadAllOfflineScaleArtifacts(config, registration)
	if err != nil {
		return VerifiedOfflineScaleReceipt{}, err
	}
	receipt, err := buildOfflineScaleReceipt(
		registration, artifacts, lastInferenceExitedAt,
		firstEvaluatorStartedAt,
	)
	if err != nil {
		return VerifiedOfflineScaleReceipt{}, err
	}
	if err := SealOfflineScaleReceipt(config.Paths().Receipt, receipt, registration); err != nil {
		return VerifiedOfflineScaleReceipt{}, err
	}
	return LoadVerifiedOfflineScaleReceipt(config)
}

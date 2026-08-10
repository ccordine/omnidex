package cognitiongauntlet

import (
	"context"
	"os"
	"path/filepath"
	"time"
)

func (inference offlinePromotionInference) evaluate(
	ctx context.Context,
) (OfflinePromotionReceipt, error) {
	if err := requireInferencePrivateOutputsAbsent(inference.paths); err != nil {
		return OfflinePromotionReceipt{}, err
	}
	temporary, err := os.MkdirTemp("", "omnidex-cognition-evaluator-")
	if err != nil {
		return OfflinePromotionReceipt{}, err
	}
	defer os.RemoveAll(temporary)
	evaluatorPath := filepath.Join(temporary, "evaluator.json")
	evaluator := newEvaluatorProcessConfig(
		inference.config, inference.paths, inference.privateOracleCredential,
		inference.executableSHA256,
	)
	if err := writePrivateProcessFile(
		evaluatorPath, evaluator, "offline evaluator process configuration",
	); err != nil {
		return OfflinePromotionReceipt{}, err
	}
	evaluatorStartedAt := time.Now().UTC()
	evaluatorPID, err := runOfflineChild(
		ctx, inference.executable, "evaluate", evaluatorPath, inference.executableSHA256,
	)
	if err != nil {
		return OfflinePromotionReceipt{}, err
	}
	evaluation, evaluationArtifactSHA256, err := LoadEvaluationArtifact(inference.paths.Evaluation)
	if err != nil {
		return OfflinePromotionReceipt{}, err
	}
	receipt, err := buildOfflinePromotionReceipt(
		inference.executionInference(), evaluation, evaluationArtifactSHA256,
		evaluatorPID, evaluatorStartedAt, time.Now().UTC(),
	)
	if err != nil {
		return OfflinePromotionReceipt{}, err
	}
	if _, err := receipt.VerifyEvaluationArtifact(inference.paths.Evaluation); err != nil {
		return OfflinePromotionReceipt{}, err
	}
	if err := sealScenarioArtifact(
		inference.paths.Receipt, receipt, "offline cognition promotion receipt",
	); err != nil {
		return OfflinePromotionReceipt{}, err
	}
	return receipt, nil
}

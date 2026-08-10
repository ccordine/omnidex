package cognitiongauntlet

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func evaluateOfflineScaleInference(
	ctx context.Context,
	config OfflineScaleConfig,
	inference offlineScaleInference,
) (OfflinePromotionReceipt, error) {
	paths := config.runPaths(inference.current)
	if err := requireInferencePrivateOutputsAbsent(paths); err != nil {
		return OfflinePromotionReceipt{}, err
	}
	if err := ensureAbsent(
		config.scaleEvidencePath(inference.current), "private Scale evaluation evidence",
	); err != nil {
		return OfflinePromotionReceipt{}, err
	}
	temporary, err := os.MkdirTemp("", "omnidex-scale-evaluator-")
	if err != nil {
		return OfflinePromotionReceipt{}, err
	}
	defer os.RemoveAll(temporary)
	process := newScaleEvaluatorProcessConfig(
		config, inference.current, inference.credential, inference.execution.executableSHA256,
	)
	processPath := filepath.Join(temporary, "scale-evaluator.json")
	if err := writePrivateProcessFile(
		processPath, process, "offline Scale evaluator process configuration",
	); err != nil {
		return OfflinePromotionReceipt{}, err
	}
	evaluatorStartedAt := time.Now().UTC()
	evaluatorPID, err := runOfflineChild(
		ctx, inference.execution.executable, "evaluate-scale", processPath,
		inference.execution.executableSHA256,
	)
	if err != nil {
		return OfflinePromotionReceipt{}, err
	}
	evaluation, evaluationSHA, err := LoadEvaluationArtifact(paths.Evaluation)
	if err != nil {
		return OfflinePromotionReceipt{}, err
	}
	evidence, _, err := LoadOfflineScaleEvaluationEvidence(
		config.scaleEvidencePath(inference.current),
	)
	if err != nil || evidence.EvaluationArtifactSHA256 != evaluationSHA {
		return OfflinePromotionReceipt{}, fmt.Errorf("Scale evaluator evidence changed evaluation: %w", err)
	}
	receipt, err := buildOfflinePromotionReceipt(
		inference.execution, evaluation, evaluationSHA, evaluatorPID,
		evaluatorStartedAt, time.Now().UTC(),
	)
	if err != nil {
		return OfflinePromotionReceipt{}, err
	}
	if err := sealScenarioArtifact(
		paths.Receipt, receipt, "offline cognition promotion receipt",
	); err != nil {
		return OfflinePromotionReceipt{}, err
	}
	return receipt, nil
}

package cognitiongauntlet

import (
	"context"
	"fmt"
	"time"
)

func finishOfflineLiveProbe(
	ctx context.Context,
	setup *offlineResumeExecution,
	episode SealedEpisode,
	replacementPID int,
	allInferenceExitedAt time.Time,
	inferenceStartedAt time.Time,
) (offlinePromotionInference, error) {
	if setup == nil || setup.database == nil || setup.host == nil || replacementPID <= 0 ||
		allInferenceExitedAt.IsZero() || inferenceStartedAt.IsZero() ||
		allInferenceExitedAt.Before(inferenceStartedAt) {
		return offlinePromotionInference{}, fmt.Errorf("live Resume completion authority is incomplete")
	}
	if err := setup.database.revokeInference(context.Background()); err != nil {
		return offlinePromotionInference{}, err
	}
	if err := setup.host.close(ctx); err != nil {
		return offlinePromotionInference{}, err
	}
	hostReceipt, err := setup.host.receipt()
	if err != nil {
		return offlinePromotionInference{}, err
	}
	if err := requireInferencePrivateOutputsAbsent(setup.paths); err != nil {
		return offlinePromotionInference{}, err
	}
	if err := validatePublicInferenceEpisode(setup.bundle, episode); err != nil {
		return offlinePromotionInference{}, err
	}
	if err := completeOfflineInferenceStep(ctx, setup.database); err != nil {
		return offlinePromotionInference{}, err
	}
	return offlinePromotionInference{
		config: setup.config, executable: setup.executable,
		executableSHA256: setup.executableSHA256, paths: setup.paths,
		bundle: setup.bundle, episode: episode,
		privateOracleCredential: setup.privateOracleCredential,
		databaseSchema:          setup.database.schema,
		generatorPID:            setup.generatorPID, generatorExitedAt: setup.generatorExitedAt,
		host: hostReceipt, inferencePID: replacementPID,
		inferenceStartedAt: inferenceStartedAt, inferenceExitedAt: allInferenceExitedAt,
	}, nil
}

func buildLiveResumeInterruption(
	run offlineLiveStaleInference,
	baseline ResumeBaselineArtifact,
) (OfflineResumeInterruptionReceipt, error) {
	checkpoint, err := baseline.checkpoint(0)
	if err != nil {
		return OfflineResumeInterruptionReceipt{}, err
	}
	checkpointSHA, err := digestJSON(checkpoint)
	if err != nil {
		return OfflineResumeInterruptionReceipt{}, err
	}
	receipt := OfflineResumeInterruptionReceipt{
		DecisionBoundary: 0, BaselineCheckpointSHA256: checkpointSHA,
		Original: run.original, Replacement: run.replacement,
		OriginalPID: run.originalPID, ReplacementPID: run.replacementPID,
		OriginalStoppedAt: run.originalStoppedAt, OriginalResumedAt: run.originalResumedAt,
		OriginalExitedAt: run.originalExitedAt, Continuity: run.continuity,
	}
	return receipt, receipt.validate(checkpoint)
}

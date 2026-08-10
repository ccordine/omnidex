package cognitiongauntlet

import (
	"context"
	"fmt"
	"reflect"
	"time"
)

func evaluateLiveResumeProbes(
	ctx context.Context,
	inference offlineResumeInference,
	baseline ResumeBaselineArtifact,
	lastInferenceExitedAt time.Time,
) (offlineResumeInference, OfflinePromotionReceipt, string, Evaluation, error) {
	if len(inference.liveProbeRuns) != len(liveStalePorts()) {
		return offlineResumeInference{}, OfflinePromotionReceipt{}, "", Evaluation{},
			fmt.Errorf("live Resume omitted a stale-port inference")
	}
	proofs := make([]LiveStalePortProof, 0, len(inference.liveProbeRuns))
	var primaryReceipt OfflinePromotionReceipt
	var primarySHA string
	var primaryEvaluation Evaluation
	for _, run := range inference.liveProbeRuns {
		receipt, err := run.promotion.evaluate(ctx)
		if err != nil {
			return offlineResumeInference{}, OfflinePromotionReceipt{}, "", Evaluation{}, err
		}
		if receipt.EvaluatorStartedAt.Before(lastInferenceExitedAt) {
			return offlineResumeInference{}, OfflinePromotionReceipt{}, "", Evaluation{},
				fmt.Errorf("live Resume evaluator started before every inference exited")
		}
		loaded, receiptSHA, err := loadOfflinePromotionReceiptArtifact(run.promotion.paths.Receipt)
		if err != nil || !reflect.DeepEqual(loaded, receipt) {
			return offlineResumeInference{}, OfflinePromotionReceipt{}, "", Evaluation{},
				fmt.Errorf("live Resume promotion receipt changed after sealing: %w", err)
		}
		evaluation, err := receipt.VerifyEvaluationArtifact(run.promotion.paths.Evaluation)
		if err != nil {
			return offlineResumeInference{}, OfflinePromotionReceipt{}, "", Evaluation{}, err
		}
		if !resumeEvaluationQualified(evaluation) {
			return offlineResumeInference{}, OfflinePromotionReceipt{}, "", Evaluation{},
				fmt.Errorf("live Resume port %q replacement was not competence-qualified", run.port)
		}
		proof, err := buildLiveStalePortProof(run, receipt, receiptSHA)
		if err != nil {
			return offlineResumeInference{}, OfflinePromotionReceipt{}, "", Evaluation{}, err
		}
		proofs = append(proofs, proof)
		if run.port == liveStalePolicyFinish {
			primaryReceipt, primarySHA, primaryEvaluation = receipt, receiptSHA, evaluation
		}
	}
	completedAt := time.Time{}
	for _, proof := range proofs {
		if proof.EvaluatorCompletedAt.After(completedAt) {
			completedAt = proof.EvaluatorCompletedAt
		}
	}
	receipt := LiveStaleProbeReceipt{
		Schema:                   LiveStaleProbeReceiptSchemaV2,
		PublicRunAuthoritySHA256: inference.promotion.episode.Manifest.PublicRunAuthoritySHA256,
		Probes:                   proofs, CompletedAt: completedAt,
	}
	if err := receipt.Validate(); err != nil {
		return offlineResumeInference{}, OfflinePromotionReceipt{}, "", Evaluation{}, err
	}
	path := resumeLiveStaleProbePath(inference.promotion.config)
	if err := SealLiveStaleProbeReceipt(path, receipt); err != nil {
		return offlineResumeInference{}, OfflinePromotionReceipt{}, "", Evaluation{}, err
	}
	probeSHA, err := hashExactFile(path, maxOfflineMatrixArtifactBytes)
	if err != nil {
		return offlineResumeInference{}, OfflinePromotionReceipt{}, "", Evaluation{}, err
	}
	inference.liveStaleProbe, inference.liveStaleProbeSHA256 = &receipt, probeSHA
	inference, err = sealOfflineResumeInferenceEvidence(
		inference, baseline, &receipt, probeSHA,
	)
	if err != nil {
		return offlineResumeInference{}, OfflinePromotionReceipt{}, "", Evaluation{}, err
	}
	return inference, primaryReceipt, primarySHA, primaryEvaluation, nil
}

func resumeEvaluationQualified(evaluation Evaluation) bool {
	return evaluation.Validate() == nil && evaluation.GoalSuccess && evaluation.ValidTerminalState &&
		evaluation.CausalAcquisition != nil && evaluation.CausalAcquisition.Validate() == nil &&
		evaluation.CausalAcquisition.AcquiredEvidence ==
			evaluation.CausalAcquisition.RequiredEvidence &&
		evaluation.CleanDesk != nil && evaluation.CleanDesk.ConcentrationQualified &&
		evaluation.CleanDesk.MissingCriticalRefs == 0 &&
		evaluation.CleanDesk.NativeUsageComplete && evaluation.CleanDesk.BudgetQualified
}

func buildLiveStalePortProof(
	run offlineLiveStaleInference,
	receipt OfflinePromotionReceipt,
	receiptSHA string,
) (LiveStalePortProof, error) {
	beforeSHA, err := run.stateBefore.SHA256()
	if err != nil {
		return LiveStalePortProof{}, err
	}
	afterSHA, err := run.stateAfter.SHA256()
	if err != nil {
		return LiveStalePortProof{}, err
	}
	proof := LiveStalePortProof{
		Port: run.port, Episode: run.stateBefore.Episode,
		EpisodeSealSHA256:      run.promotion.episode.SealSHA256,
		EvaluationSHA256:       receipt.EvaluationArtifactSHA256,
		PromotionReceiptSHA256: receiptSHA,
		DatabaseSchema:         run.promotion.databaseSchema, HostSchema: run.hostSchema,
		Original: run.original, Replacement: run.replacement,
		OriginalPID: run.originalPID, ReplacementPID: run.replacementPID,
		Checkpoint: run.checkpoint, Rejection: run.rejection,
		StateBefore: run.stateBefore, StateAfter: run.stateAfter,
		StateBeforeSHA256: beforeSHA, StateAfterSHA256: afterSHA,
		ReplacementSealedAt:  run.replacementSealedAt,
		OriginalResumedAt:    run.originalResumedAt,
		InferenceStartedAt:   receipt.InferenceStartedAt,
		InferenceExitedAt:    receipt.InferenceExitedAt,
		EvaluatorStartedAt:   receipt.EvaluatorStartedAt,
		EvaluatorCompletedAt: receipt.CompletedAt,
	}
	return proof, proof.Validate()
}

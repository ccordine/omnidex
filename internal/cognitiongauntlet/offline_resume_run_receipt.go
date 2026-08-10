package cognitiongauntlet

import "fmt"

func buildOfflineResumeRunReceipt(
	inference offlineResumeInference,
	promotion OfflinePromotionReceipt,
	promotionSHA256 string,
	evaluation Evaluation,
	baseline ResumeBaselineArtifact,
) (OfflineResumeRunReceipt, error) {
	evidence, err := buildResumeScheduleEvidence(
		inference, baseline, inference.liveStaleProbe, inference.liveStaleProbeSHA256,
	)
	if err != nil {
		return OfflineResumeRunReceipt{}, err
	}
	return composeOfflineResumeRunReceipt(
		evidence, inference.scheduleEvidenceSHA256, promotion, promotionSHA256,
		inference.promotion.episode, evaluation, baseline,
	)
}

func composeOfflineResumeRunReceipt(
	evidence ResumeScheduleEvidenceArtifact,
	evidenceSHA256 string,
	promotion OfflinePromotionReceipt,
	promotionSHA256 string,
	episode SealedEpisode,
	evaluation Evaluation,
	baseline ResumeBaselineArtifact,
) (OfflineResumeRunReceipt, error) {
	if err := promotion.Validate(); err != nil {
		return OfflineResumeRunReceipt{}, err
	}
	if err := evaluation.Validate(); err != nil {
		return OfflineResumeRunReceipt{}, err
	}
	if err := episode.Validate(); err != nil {
		return OfflineResumeRunReceipt{}, err
	}
	if err := evidence.Validate(baseline); err != nil {
		return OfflineResumeRunReceipt{}, err
	}
	if !validDigest(promotionSHA256) || !validDigest(evidenceSHA256) ||
		promotion.EpisodeSealSHA256 != episode.SealSHA256 ||
		evaluation.EpisodeSealSHA256 != episode.SealSHA256 ||
		evidence.EpisodeSealSHA256 != episode.SealSHA256 ||
		evidence.PublicRunAuthoritySHA256 != promotion.PublicRunAuthoritySHA256 {
		return OfflineResumeRunReceipt{}, fmt.Errorf("Resume run artifacts changed authority")
	}
	semantics, err := DeriveResumeEpisodeSemantics(episode)
	if err != nil {
		return OfflineResumeRunReceipt{}, err
	}
	if semantics != evidence.Semantics {
		return OfflineResumeRunReceipt{}, fmt.Errorf("Resume schedule evidence changed sealed episode semantics")
	}
	causalComplete := evaluation.CausalAcquisition != nil &&
		evaluation.CausalAcquisition.Validate() == nil &&
		evaluation.CausalAcquisition.AcquiredEvidence ==
			evaluation.CausalAcquisition.RequiredEvidence
	cleanDeskQualified := evaluation.CleanDesk != nil &&
		evaluation.CleanDesk.ConcentrationQualified &&
		evaluation.CleanDesk.MissingCriticalRefs == 0 &&
		evaluation.CleanDesk.NativeUsageComplete && evaluation.CleanDesk.BudgetQualified
	run := OfflineResumeRunReceipt{
		Schedule: evidence.Schedule, ScheduleEvidenceSHA256: evidenceSHA256,
		PromotionReceiptSHA256:   promotionSHA256,
		PublicRunAuthoritySHA256: promotion.PublicRunAuthoritySHA256,
		EpisodeSealSHA256:        promotion.EpisodeSealSHA256,
		EvaluationArtifactSHA256: promotion.EvaluationArtifactSHA256,
		Semantics:                semantics, Interruptions: append([]OfflineResumeInterruptionReceipt{}, evidence.Interruptions...),
		LiveStaleProbe: evidence.LiveStaleProbe, LiveStaleProbeSHA256: evidence.LiveStaleProbeSHA256,
		GoalSuccess: evaluation.GoalSuccess, ValidTerminalState: evaluation.ValidTerminalState,
		CausalAdmissionComplete: causalComplete, CleanDeskQualified: cleanDeskQualified,
		Recovery:             evidence.Recovery,
		InferenceStartedAt:   promotion.InferenceStartedAt,
		InferenceExitedAt:    promotion.InferenceExitedAt,
		EvaluatorStartedAt:   promotion.EvaluatorStartedAt,
		EvaluatorCompletedAt: promotion.CompletedAt,
	}
	if run.LiveStaleProbe != nil && run.LiveStaleProbe.CompletedAt.After(run.EvaluatorCompletedAt) {
		run.EvaluatorCompletedAt = run.LiveStaleProbe.CompletedAt
	}
	return run, run.validate(evidence.Schedule, baseline)
}

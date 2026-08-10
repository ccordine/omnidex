package cognitiongauntlet

import (
	"fmt"
	"reflect"
)

func verifyOfflineResumeRun(
	config OfflineResumeConfig,
	registration OfflineResumePreregistration,
	schedule OfflineResumeSchedule,
	baseline ResumeBaselineArtifact,
) (OfflineResumeRunReceipt, error) {
	runConfig, err := config.derivedRunConfig(registration, schedule)
	if err != nil {
		return OfflineResumeRunReceipt{}, err
	}
	paths := runConfig.Paths()
	bundle, err := LoadPublicInferenceBundle(paths.PublicBundle)
	if err != nil {
		return OfflineResumeRunReceipt{}, err
	}
	publicSHA, err := bundle.Authority.SHA256()
	if err != nil {
		return OfflineResumeRunReceipt{}, err
	}
	if publicSHA != baseline.PublicRunAuthoritySHA256 || bundle.Authority.Variant != VariantFullCognition ||
		bundle.Authority.RatGeneration != config.RatGeneration ||
		bundle.Authority.Budget != config.Budget || bundle.Authority.Repetition != config.Plan.Repetition {
		return OfflineResumeRunReceipt{}, fmt.Errorf("Resume run changed its frozen public authority")
	}
	episode, err := LoadSealedEpisode(paths.Episode)
	if err != nil {
		return OfflineResumeRunReceipt{}, err
	}
	if err := validatePublicInferenceEpisode(bundle, episode); err != nil {
		return OfflineResumeRunReceipt{}, err
	}
	promotion, promotionSHA, err := loadOfflinePromotionReceiptArtifact(paths.Receipt)
	if err != nil {
		return OfflineResumeRunReceipt{}, err
	}
	evaluation, err := promotion.VerifyEvaluationArtifact(paths.Evaluation)
	if err != nil {
		return OfflineResumeRunReceipt{}, err
	}
	if evaluation.Seed != config.Plan.Seed ||
		evaluation.TaskArchetype != offlineScenarioTaskArchetype(SuiteCombined) {
		return OfflineResumeRunReceipt{}, fmt.Errorf("Resume evaluation changed private workload authority")
	}
	evidence, evidenceSHA, err := LoadResumeScheduleEvidence(
		resumeScheduleEvidencePath(runConfig), baseline,
	)
	if err != nil {
		return OfflineResumeRunReceipt{}, err
	}
	if evidence.Schedule.ID != schedule.ID || evidence.Schedule.Kind != schedule.Kind {
		return OfflineResumeRunReceipt{}, fmt.Errorf("Resume schedule evidence belongs to another schedule")
	}
	if schedule.Kind == ResumeLiveInferenceExpiry {
		probe, err := LoadLiveStaleProbeReceipt(resumeLiveStaleProbePath(runConfig))
		if err != nil {
			return OfflineResumeRunReceipt{}, err
		}
		probeSHA, err := hashExactFile(
			resumeLiveStaleProbePath(runConfig), maxOfflineMatrixArtifactBytes,
		)
		if err != nil {
			return OfflineResumeRunReceipt{}, err
		}
		if !reflect.DeepEqual(evidence.LiveStaleProbe, &probe) ||
			evidence.LiveStaleProbeSHA256 != probeSHA {
			return OfflineResumeRunReceipt{}, fmt.Errorf("Resume live stale-probe artifact changed")
		}
		if err := verifyLiveStaleProbeArtifacts(config, runConfig, probe); err != nil {
			return OfflineResumeRunReceipt{}, err
		}
	} else if evidence.LiveStaleProbe != nil || evidence.LiveStaleProbeSHA256 != "" {
		return OfflineResumeRunReceipt{}, fmt.Errorf("ordinary Resume run carries live stale-probe evidence")
	}
	return composeOfflineResumeRunReceipt(
		evidence, evidenceSHA, promotion, promotionSHA, episode, evaluation, baseline,
	)
}

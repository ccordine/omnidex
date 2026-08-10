package cognitiongauntlet

import (
	"fmt"
	"path/filepath"
)

const ResumeScheduleEvidenceSchemaV1 = "omnidex.resume-schedule-evidence.v1"

type ResumeScheduleEvidenceArtifact struct {
	Schema                   string                             `json:"schema"`
	Schedule                 OfflineResumeSchedule              `json:"schedule"`
	PublicRunAuthoritySHA256 string                             `json:"public_run_authority_sha256"`
	EpisodeSealSHA256        string                             `json:"episode_seal_sha256"`
	Semantics                ResumeEpisodeSemantics             `json:"episode_semantics"`
	Recovery                 RecoveryMetrics                    `json:"recovery"`
	Interruptions            []OfflineResumeInterruptionReceipt `json:"interruptions"`
	LiveStaleProbe           *LiveStaleProbeReceipt             `json:"live_stale_probe,omitempty"`
	LiveStaleProbeSHA256     string                             `json:"live_stale_probe_sha256,omitempty"`
}

func buildResumeScheduleEvidence(
	inference offlineResumeInference,
	baseline ResumeBaselineArtifact,
	liveProbe *LiveStaleProbeReceipt,
	liveProbeSHA256 string,
) (ResumeScheduleEvidenceArtifact, error) {
	semantics, err := DeriveResumeEpisodeSemantics(inference.promotion.episode)
	if err != nil {
		return ResumeScheduleEvidenceArtifact{}, err
	}
	interruptions := make([]OfflineResumeInterruptionReceipt, len(inference.interruptions))
	for index, interruption := range inference.interruptions {
		interruptions[index], err = buildOfflineResumeInterruption(interruption, baseline)
		if err != nil {
			return ResumeScheduleEvidenceArtifact{}, err
		}
	}
	if inference.liveInterruption != nil {
		if len(interruptions) != 0 || inference.schedule.Kind != ResumeLiveInferenceExpiry {
			return ResumeScheduleEvidenceArtifact{}, fmt.Errorf("live Resume interruption mixed with killed schedules")
		}
		interruptions = []OfflineResumeInterruptionReceipt{*inference.liveInterruption}
	}
	recovery := RecoveryMetrics{Restarts: len(interruptions)}
	if liveProbe != nil {
		recovery.StaleAttemptRejections = liveStaleWriteClassCount(*liveProbe)
	}
	artifact := ResumeScheduleEvidenceArtifact{
		Schema: ResumeScheduleEvidenceSchemaV1, Schedule: inference.schedule,
		PublicRunAuthoritySHA256: inference.promotion.episode.Manifest.PublicRunAuthoritySHA256,
		EpisodeSealSHA256:        inference.promotion.episode.SealSHA256,
		Semantics:                semantics, Recovery: recovery,
		Interruptions: interruptions, LiveStaleProbe: liveProbe,
		LiveStaleProbeSHA256: liveProbeSHA256,
	}
	return artifact, artifact.Validate(baseline)
}

func (artifact ResumeScheduleEvidenceArtifact) Validate(
	baseline ResumeBaselineArtifact,
) error {
	if artifact.Schema != ResumeScheduleEvidenceSchemaV1 ||
		!validDigest(artifact.PublicRunAuthoritySHA256) || !validDigest(artifact.EpisodeSealSHA256) ||
		artifact.Semantics.Validate() != nil || artifact.Interruptions == nil {
		return fmt.Errorf("Resume schedule evidence authority is invalid")
	}
	run := OfflineResumeRunReceipt{
		Schedule: artifact.Schedule, Semantics: artifact.Semantics,
		Interruptions: artifact.Interruptions, LiveStaleProbe: artifact.LiveStaleProbe,
		LiveStaleProbeSHA256: artifact.LiveStaleProbeSHA256, Recovery: artifact.Recovery,
	}
	return validateResumeInterruptions(run, baseline)
}

func resumeScheduleEvidencePath(config OfflinePromotionConfig) string {
	return filepath.Join(config.PrivateOutputDirectory, "resume-schedule-evidence.json")
}

func resumeLiveStaleProbePath(config OfflinePromotionConfig) string {
	return filepath.Join(config.PrivateOutputDirectory, "resume-live-stale-probe.json")
}

func SealResumeScheduleEvidence(
	path string,
	artifact ResumeScheduleEvidenceArtifact,
	baseline ResumeBaselineArtifact,
) error {
	if err := artifact.Validate(baseline); err != nil {
		return err
	}
	return sealScenarioArtifact(path, artifact, "Resume schedule evidence")
}

func LoadResumeScheduleEvidence(
	path string,
	baseline ResumeBaselineArtifact,
) (ResumeScheduleEvidenceArtifact, string, error) {
	var artifact ResumeScheduleEvidenceArtifact
	if err := loadStrictJSONFile(path, &artifact, "Resume schedule evidence"); err != nil {
		return ResumeScheduleEvidenceArtifact{}, "", err
	}
	if err := artifact.Validate(baseline); err != nil {
		return ResumeScheduleEvidenceArtifact{}, "", err
	}
	digest, err := hashExactFile(path, maxOfflineMatrixArtifactBytes)
	if err != nil {
		return ResumeScheduleEvidenceArtifact{}, "", err
	}
	return artifact, digest, nil
}

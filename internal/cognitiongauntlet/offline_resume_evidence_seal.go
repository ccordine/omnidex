package cognitiongauntlet

import "fmt"

func sealOfflineResumeInferenceEvidence(
	inference offlineResumeInference,
	baseline ResumeBaselineArtifact,
	liveProbe *LiveStaleProbeReceipt,
	liveProbeSHA256 string,
) (offlineResumeInference, error) {
	if inference.scheduleEvidenceSHA256 != "" {
		return offlineResumeInference{}, fmt.Errorf("Resume schedule evidence is already sealed")
	}
	artifact, err := buildResumeScheduleEvidence(
		inference, baseline, liveProbe, liveProbeSHA256,
	)
	if err != nil {
		return offlineResumeInference{}, err
	}
	path := resumeScheduleEvidencePath(inference.promotion.config)
	if err := SealResumeScheduleEvidence(path, artifact, baseline); err != nil {
		return offlineResumeInference{}, err
	}
	loaded, digest, err := LoadResumeScheduleEvidence(path, baseline)
	if err != nil {
		return offlineResumeInference{}, err
	}
	if loaded.EpisodeSealSHA256 != inference.promotion.episode.SealSHA256 ||
		loaded.PublicRunAuthoritySHA256 !=
			inference.promotion.episode.Manifest.PublicRunAuthoritySHA256 {
		return offlineResumeInference{}, fmt.Errorf("Resume schedule evidence changed run authority")
	}
	inference.scheduleEvidenceSHA256 = digest
	return inference, nil
}

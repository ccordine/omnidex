package cognitiongauntlet

import (
	"context"
	"fmt"
	"os"
	"time"

	buildversion "github.com/gryph/omnidex/internal/version"
)

func runOfflineResumeLiveInference(
	ctx context.Context,
	config OfflinePromotionConfig,
	schedule OfflineResumeSchedule,
	executable string,
	baseline ResumeBaselineArtifact,
) (offlineResumeInference, error) {
	if ctx == nil || schedule.Kind != ResumeLiveInferenceExpiry || schedule.RequiredKills != 1 {
		return offlineResumeInference{}, fmt.Errorf("live Resume runner requires its exact schedule")
	}
	if err := config.Validate(); err != nil {
		return offlineResumeInference{}, err
	}
	if err := baseline.Validate(); err != nil {
		return offlineResumeInference{}, err
	}
	executableSHA, err := validateOfflinePromotionIdentity(
		config, executable, buildversion.Commit, buildversion.SourceSHA256,
		buildversion.MigrationsSHA256, buildversion.Version,
	)
	if err != nil {
		return offlineResumeInference{}, err
	}
	migrations, err := loadReleaseMigrationBundle(executable, buildversion.MigrationsSHA256)
	if err != nil {
		return offlineResumeInference{}, err
	}
	runs := make([]offlineLiveStaleInference, 0, len(liveStalePorts()))
	for _, port := range liveStalePorts() {
		probeConfig, err := liveStaleProbeConfig(config, port)
		if err != nil {
			return offlineResumeInference{}, err
		}
		temporary, err := os.MkdirTemp("", "omnidex-resume-live-"+string(port)+"-")
		if err != nil {
			return offlineResumeInference{}, err
		}
		run, runErr := runOfflineLiveStalePort(
			ctx, probeConfig, port, executable, executableSHA, migrations,
			temporary,
		)
		removeErr := os.RemoveAll(temporary)
		if runErr != nil || removeErr != nil {
			return offlineResumeInference{}, joinLiveRunErrors(port, runErr, removeErr)
		}
		runs = append(runs, run)
	}
	primary := runs[0]
	publicSHA := primary.promotion.episode.Manifest.PublicRunAuthoritySHA256
	for _, run := range runs {
		if run.port == liveStalePolicyFinish {
			primary = run
		}
		if run.promotion.episode.Manifest.PublicRunAuthoritySHA256 != publicSHA {
			return offlineResumeInference{}, fmt.Errorf("live Resume probes changed public authority")
		}
	}
	interruption, err := buildLiveResumeInterruption(primary, baseline)
	if err != nil {
		return offlineResumeInference{}, err
	}
	return offlineResumeInference{
		promotion: primary.promotion, schedule: schedule,
		liveInterruption: &interruption, liveProbeRuns: runs,
	}, nil
}

func joinLiveRunErrors(port liveStalePort, runErr, removeErr error) error {
	if runErr != nil {
		return fmt.Errorf("execute live Resume port %q: %w", port, runErr)
	}
	return fmt.Errorf("remove live Resume port %q workspace: %w", port, removeErr)
}

func latestTime(values ...time.Time) time.Time {
	latest := time.Time{}
	for _, value := range values {
		if value.After(latest) {
			latest = value
		}
	}
	return latest
}

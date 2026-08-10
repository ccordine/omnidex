package cognitiongauntlet

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	buildversion "github.com/gryph/omnidex/internal/version"
)

func runOfflineResumeUninterruptedInference(
	ctx context.Context,
	config OfflinePromotionConfig,
	schedule OfflineResumeSchedule,
	executable string,
) (offlineResumeInference, ResumeBaselineArtifact, string, error) {
	if schedule.Kind != ResumeUninterrupted {
		return offlineResumeInference{}, ResumeBaselineArtifact{}, "",
			fmt.Errorf("uninterrupted Resume runner received another schedule")
	}
	if err := config.Validate(); err != nil {
		return offlineResumeInference{}, ResumeBaselineArtifact{}, "", err
	}
	executableSHA, err := validateOfflinePromotionIdentity(
		config, executable, buildversion.Commit, buildversion.SourceSHA256,
		buildversion.MigrationsSHA256, buildversion.Version,
	)
	if err != nil {
		return offlineResumeInference{}, ResumeBaselineArtifact{}, "", err
	}
	migrations, err := loadReleaseMigrationBundle(executable, buildversion.MigrationsSHA256)
	if err != nil {
		return offlineResumeInference{}, ResumeBaselineArtifact{}, "", err
	}
	temporary, err := os.MkdirTemp("", "omnidex-resume-baseline-")
	if err != nil {
		return offlineResumeInference{}, ResumeBaselineArtifact{}, "", err
	}
	defer os.RemoveAll(temporary)
	checkpointDirectory := filepath.Join(temporary, "checkpoints")
	if err := os.Mkdir(checkpointDirectory, 0o700); err != nil {
		return offlineResumeInference{}, ResumeBaselineArtifact{}, "", err
	}
	setup, err := startOfflineResumeExecution(
		ctx, config, executable, executableSHA, migrations, temporary,
	)
	if err != nil {
		return offlineResumeInference{}, ResumeBaselineArtifact{}, "", err
	}
	defer setup.close()
	control, err := resumeBaselineInferenceControl(checkpointDirectory)
	if err != nil {
		return offlineResumeInference{}, ResumeBaselineArtifact{}, "", err
	}
	process := newInferenceProcessConfig(
		config, setup.database, setup.host, setup.paths, executableSHA, "",
	)
	process.Control = control
	processPath := filepath.Join(temporary, "baseline-inference.json")
	if err := writePrivateProcessFile(
		processPath, process, "Resume baseline inference configuration",
	); err != nil {
		return offlineResumeInference{}, ResumeBaselineArtifact{}, "", err
	}
	if err := setup.database.enableInference(ctx); err != nil {
		return offlineResumeInference{}, ResumeBaselineArtifact{}, "", err
	}
	pid, err := runOfflineChild(ctx, executable, "infer", processPath, executableSHA)
	exitedAt := time.Now().UTC()
	if err != nil {
		return offlineResumeInference{}, ResumeBaselineArtifact{}, "", err
	}
	inference, err := finishOfflineResumeExecution(
		ctx, setup, schedule, []takeoverInterruption{}, pid, exitedAt,
	)
	if err != nil {
		return offlineResumeInference{}, ResumeBaselineArtifact{}, "", err
	}
	checkpoints, err := loadResumeBaselineCheckpoints(
		checkpointDirectory, config.Scenario.Budget().ModelCalls,
	)
	if err != nil {
		return offlineResumeInference{}, ResumeBaselineArtifact{}, "", err
	}
	publicSHA, err := setup.bundle.Authority.SHA256()
	if err != nil {
		return offlineResumeInference{}, ResumeBaselineArtifact{}, "", err
	}
	baseline, err := NewResumeBaselineArtifact(publicSHA, inference.promotion.episode, checkpoints)
	if err != nil {
		return offlineResumeInference{}, ResumeBaselineArtifact{}, "", err
	}
	baselinePath := filepath.Join(config.PrivateOutputDirectory, "resume-baseline.json")
	if err := SealResumeBaselineArtifact(baselinePath, baseline); err != nil {
		return offlineResumeInference{}, ResumeBaselineArtifact{}, "", err
	}
	baselineSHA, err := hashExactFile(baselinePath, maxOfflineMatrixArtifactBytes)
	if err != nil {
		return offlineResumeInference{}, ResumeBaselineArtifact{}, "", err
	}
	inference, err = sealOfflineResumeInferenceEvidence(inference, baseline, nil, "")
	if err != nil {
		return offlineResumeInference{}, ResumeBaselineArtifact{}, "", err
	}
	return inference, baseline, baselineSHA, nil
}

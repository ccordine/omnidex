package cognitiongauntlet

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"time"

	buildversion "github.com/gryph/omnidex/internal/version"
)

func runOfflineResumeEveryDecisionInference(
	ctx context.Context,
	config OfflinePromotionConfig,
	schedule OfflineResumeSchedule,
	executable string,
) (offlineResumeInference, error) {
	if schedule.Kind != ResumeEveryDecision || !schedule.Dynamic {
		return offlineResumeInference{}, fmt.Errorf("dynamic Resume runner received another schedule")
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
	temporary, err := os.MkdirTemp("", "omnidex-resume-every-decision-")
	if err != nil {
		return offlineResumeInference{}, err
	}
	defer os.RemoveAll(temporary)
	setup, err := startOfflineResumeExecution(
		ctx, config, executable, executableSHA, migrations, temporary,
	)
	if err != nil {
		return offlineResumeInference{}, err
	}
	defer setup.close()
	boundary := inferenceBoundary{Kind: inferenceBoundaryDecisions, Count: 1}
	before, sourcePID, killedAt, err := runKilledInferenceAtBoundary(
		ctx, config, setup.database, setup.host, setup.paths, executable, executableSHA,
		boundary, temporary, setup.bundle, "every-source",
	)
	if err != nil {
		return offlineResumeInference{}, err
	}
	sourceAttempt := setup.database.attempt
	interruptions := make([]takeoverInterruption, 0, config.Scenario.Budget().ModelCalls)
	for nextCount := uint32(2); nextCount <= uint32(config.Scenario.Budget().ModelCalls); nextCount++ {
		replacement, err := claimResumeReplacement(ctx, setup.database, sourceAttempt)
		if err != nil {
			return offlineResumeInference{}, err
		}
		setup.database.attempt = replacement
		after, stopped, replacementPID, exitedAt, terminal, err := runDynamicResumeReplacement(
			ctx, config, setup, executable, executableSHA, temporary,
			before, inferenceBoundary{Kind: inferenceBoundaryDecisions, Count: nextCount},
			len(interruptions)+1,
		)
		if err != nil {
			return offlineResumeInference{}, err
		}
		interruptions = append(interruptions, takeoverInterruption{
			Boundary: before.Boundary, Before: before, After: after,
			Original: sourceAttempt, Replacement: replacement,
			OriginalPID: sourcePID, ReplacementPID: replacementPID, OriginalDied: killedAt,
		})
		if terminal {
			return finishOfflineResumeExecution(
				ctx, setup, schedule, interruptions, replacementPID, exitedAt,
			)
		}
		sourceAttempt, sourcePID, killedAt = replacement, replacementPID, exitedAt
		before = stopped
	}
	return offlineResumeInference{}, fmt.Errorf("every-decision Resume exhausted its frozen model-call budget")
}

func runDynamicResumeReplacement(
	ctx context.Context,
	config OfflinePromotionConfig,
	setup *offlineResumeExecution,
	executable string,
	executableSHA string,
	temporary string,
	resume PausedInferenceCheckpoint,
	next inferenceBoundary,
	segment int,
) (PausedInferenceCheckpoint, PausedInferenceCheckpoint, int, time.Time, bool, error) {
	label := fmt.Sprintf("every-%03d", segment)
	sourcePath := takeoverProcessPath(temporary, label+"-source")
	verificationPath := takeoverProcessPath(temporary, label+"-verification")
	nextPath := takeoverProcessPath(temporary, label+"-next")
	if err := SealPausedInferenceCheckpoint(sourcePath, resume); err != nil {
		return PausedInferenceCheckpoint{}, PausedInferenceCheckpoint{}, 0, time.Time{}, false, err
	}
	control, err := newChainedReplacementInferenceControl(
		next, nextPath, sourcePath, verificationPath, resume.Boundary,
	)
	if err != nil {
		return PausedInferenceCheckpoint{}, PausedInferenceCheckpoint{}, 0, time.Time{}, false, err
	}
	process := newInferenceProcessConfig(config, setup.database, setup.host, setup.paths, executableSHA, "")
	process.Control = control
	processPath := takeoverProcessPath(temporary, label+"-process")
	if err := writePrivateProcessFile(processPath, process, "every-decision process configuration"); err != nil {
		return PausedInferenceCheckpoint{}, PausedInferenceCheckpoint{}, 0, time.Time{}, false, err
	}
	if err := setup.database.enableInference(ctx); err != nil {
		return PausedInferenceCheckpoint{}, PausedInferenceCheckpoint{}, 0, time.Time{}, false, err
	}
	child, err := startOfflineChild(ctx, executable, "infer", processPath, executableSHA)
	if err != nil {
		return PausedInferenceCheckpoint{}, PausedInferenceCheckpoint{}, 0, time.Time{}, false, err
	}
	defer child.signal(syscall.SIGKILL)
	after, err := waitForPausedInference(
		ctx, setup.database.pool, setup.database.repository, resume.Episode.ID,
		setup.database.attempt, resume.Boundary, verificationPath, child,
	)
	if err != nil {
		return PausedInferenceCheckpoint{}, PausedInferenceCheckpoint{}, 0, time.Time{}, false, err
	}
	if _, err := NewTakeoverContinuityProof(resume.PreCall, after.PreCall); err != nil {
		return PausedInferenceCheckpoint{}, PausedInferenceCheckpoint{}, 0, time.Time{}, false, err
	}
	if err := child.signal(syscall.SIGCONT); err != nil {
		return PausedInferenceCheckpoint{}, PausedInferenceCheckpoint{}, 0, time.Time{}, false, err
	}
	stopped, terminal, exitedAt, err := waitDynamicResumeBoundary(
		ctx, setup, child, next, nextPath,
	)
	return after, stopped, child.pid(), exitedAt, terminal, err
}

func waitDynamicResumeBoundary(
	ctx context.Context,
	setup *offlineResumeExecution,
	child *offlineChildProcess,
	boundary inferenceBoundary,
	checkpointPath string,
) (PausedInferenceCheckpoint, bool, time.Time, error) {
	wait := make(chan error, 1)
	go func() {
		_, err := child.wait()
		wait <- err
	}()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return PausedInferenceCheckpoint{}, false, time.Time{}, ctx.Err()
		case err := <-wait:
			exitedAt := time.Now().UTC()
			if err != nil {
				return PausedInferenceCheckpoint{}, false, time.Time{}, err
			}
			if _, statErr := os.Stat(setup.paths.Episode); statErr != nil {
				return PausedInferenceCheckpoint{}, false, time.Time{},
					fmt.Errorf("dynamic Resume inference exited without a sealed episode: %w", statErr)
			}
			return PausedInferenceCheckpoint{}, true, exitedAt, nil
		case <-ticker.C:
			if _, err := os.Stat(checkpointPath); os.IsNotExist(err) {
				continue
			} else if err != nil {
				return PausedInferenceCheckpoint{}, false, time.Time{}, err
			}
			checkpoint, err := LoadPausedInferenceCheckpoint(checkpointPath)
			if err != nil {
				return PausedInferenceCheckpoint{}, false, time.Time{}, err
			}
			if checkpoint.Boundary != boundary {
				return PausedInferenceCheckpoint{}, false, time.Time{},
					fmt.Errorf("dynamic Resume checkpoint changed authority")
			}
			if err := requireDurablePausedProjection(
				ctx, setup.database.repository, setup.database.attempt, checkpoint,
			); err != nil {
				return PausedInferenceCheckpoint{}, false, time.Time{}, err
			}
			if err := child.signal(syscall.SIGKILL); err != nil {
				return PausedInferenceCheckpoint{}, false, time.Time{}, err
			}
			if err := <-wait; err == nil {
				return PausedInferenceCheckpoint{}, false, time.Time{},
					fmt.Errorf("dynamic Resume boundary child was not killed")
			}
			if err := setup.database.revokeInference(context.Background()); err != nil {
				return PausedInferenceCheckpoint{}, false, time.Time{}, err
			}
			if err := ensureAbsent(setup.paths.Episode, "dynamic Resume interrupted episode seal"); err != nil {
				return PausedInferenceCheckpoint{}, false, time.Time{}, err
			}
			return checkpoint, false, time.Now().UTC(), nil
		}
	}
}

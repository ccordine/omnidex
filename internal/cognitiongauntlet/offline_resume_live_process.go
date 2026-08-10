package cognitiongauntlet

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

type liveChildExit struct {
	pid int
	err error
	at  time.Time
}

func runOfflineLiveStalePort(
	ctx context.Context,
	config OfflinePromotionConfig,
	port liveStalePort,
	executable string,
	executableSHA string,
	migrations queue.MigrationBundle,
	temporary string,
) (offlineLiveStaleInference, error) {
	if port.Validate() != nil {
		return offlineLiveStaleInference{}, fmt.Errorf("live Resume config does not identify a probe port")
	}
	setup, err := startOfflineResumeExecution(
		ctx, config, executable, executableSHA, migrations, temporary,
	)
	if err != nil {
		return offlineLiveStaleInference{}, err
	}
	defer setup.close()
	paths := liveProbeProcessPaths(temporary, port)
	control, err := liveStalePortInferenceControl(
		port, paths.checkpoint, paths.rejection, paths.generation,
	)
	if err != nil {
		return offlineLiveStaleInference{}, err
	}
	original := setup.database.attempt
	old, exits, startedAt, err := startLiveProbeChild(
		ctx, setup, control, paths.process,
	)
	if err != nil {
		return offlineLiveStaleInference{}, err
	}
	defer old.signal(syscall.SIGKILL)
	stoppedAt, err := waitForLiveProbePause(ctx, port, paths, old, exits, original)
	if err != nil {
		return offlineLiveStaleInference{}, err
	}
	before, err := captureLivePreCall(ctx, setup, original)
	if err != nil {
		return offlineLiveStaleInference{}, err
	}
	replacement, err := claimResumeReplacement(ctx, setup.database, original)
	if err != nil {
		return offlineLiveStaleInference{}, err
	}
	setup.database.attempt = replacement
	after, err := captureLivePreCall(ctx, setup, replacement)
	if err != nil {
		return offlineLiveStaleInference{}, err
	}
	continuity, err := NewTakeoverContinuityProof(before, after)
	if err != nil {
		return offlineLiveStaleInference{}, err
	}
	replacementPID, replacementExitedAt, err := runLiveReplacement(
		ctx, setup, port, paths.replacement,
	)
	if err != nil {
		return offlineLiveStaleInference{}, err
	}
	episode, err := LoadSealedEpisode(setup.paths.Episode)
	if err != nil {
		return offlineLiveStaleInference{}, err
	}
	stateBefore, err := captureLiveStaleDurableState(ctx, setup.database, episode)
	if err != nil {
		return offlineLiveStaleInference{}, err
	}
	resumedAt := time.Now().UTC()
	if err := old.signal(syscall.SIGCONT); err != nil {
		return offlineLiveStaleInference{}, fmt.Errorf("resume stale %q process: %w", port, err)
	}
	oldExit, err := waitForExpectedLiveRejection(ctx, exits)
	if err != nil {
		return offlineLiveStaleInference{}, err
	}
	checkpoint, err := loadLiveStalePortCheckpoint(paths.checkpoint)
	if err != nil {
		return offlineLiveStaleInference{}, err
	}
	rejection, err := loadLiveStalePortRejection(paths.rejection)
	if err != nil || rejection.ValidateFor(checkpoint) != nil {
		return offlineLiveStaleInference{}, fmt.Errorf("load exact stale %q rejection: %w", port, err)
	}
	stateAfter, err := captureLiveStaleDurableState(ctx, setup.database, episode)
	if err != nil {
		return offlineLiveStaleInference{}, err
	}
	promotion, err := finishOfflineLiveProbe(
		ctx, setup, episode, replacementPID,
		latestTime(replacementExitedAt, oldExit.at), startedAt,
	)
	if err != nil {
		return offlineLiveStaleInference{}, err
	}
	return offlineLiveStaleInference{
		port: port, promotion: promotion, original: original, replacement: replacement,
		originalPID: old.pid(), replacementPID: replacementPID,
		checkpoint: checkpoint, rejection: rejection,
		stateBefore: stateBefore, stateAfter: stateAfter,
		replacementSealedAt: replacementExitedAt, originalResumedAt: resumedAt,
		originalStoppedAt: stoppedAt, originalExitedAt: oldExit.at, continuity: continuity,
		hostSchema: setup.database.hostSchema,
	}, nil
}

type liveProbePaths struct {
	process, replacement, checkpoint, rejection, generation string
}

func liveProbeProcessPaths(directory string, port liveStalePort) liveProbePaths {
	prefix := filepath.Join(directory, string(port))
	paths := liveProbePaths{
		process: prefix + "-old.json", replacement: prefix + "-replacement.json",
		checkpoint: prefix + "-checkpoint.json", rejection: prefix + "-rejection.json",
	}
	if port == liveStalePolicyFinish {
		paths.generation = prefix + "-generation.json"
	}
	return paths
}

func startLiveProbeChild(
	ctx context.Context,
	setup *offlineResumeExecution,
	control inferenceProcessControl,
	processPath string,
) (*offlineChildProcess, <-chan liveChildExit, time.Time, error) {
	process := newInferenceProcessConfig(
		setup.config, setup.database, setup.host, setup.paths, setup.executableSHA256, "",
	)
	process.Control = control
	if err := writePrivateProcessFile(processPath, process, "live Resume probe process"); err != nil {
		return nil, nil, time.Time{}, err
	}
	if err := setup.database.enableInference(ctx); err != nil {
		return nil, nil, time.Time{}, err
	}
	startedAt := time.Now().UTC()
	child, err := startOfflineChild(
		ctx, setup.executable, "infer", processPath, setup.executableSHA256,
	)
	if err != nil {
		return nil, nil, time.Time{}, err
	}
	exits := make(chan liveChildExit, 1)
	go func() {
		pid, waitErr := child.wait()
		exits <- liveChildExit{pid: pid, err: waitErr, at: time.Now().UTC()}
	}()
	return child, exits, startedAt, nil
}

func waitForLiveProbePause(
	ctx context.Context,
	port liveStalePort,
	paths liveProbePaths,
	child *offlineChildProcess,
	exits <-chan liveChildExit,
	original model.StepAttemptAuthority,
) (time.Time, error) {
	path := paths.checkpoint
	if port == liveStalePolicyFinish {
		path = paths.generation
	}
	if err := waitForLiveArtifact(ctx, path, exits); err != nil {
		return time.Time{}, err
	}
	if err := child.signal(syscall.SIGSTOP); err != nil {
		return time.Time{}, err
	}
	if port == liveStalePolicyFinish {
		checkpoint, err := LoadLiveGenerationCheckpoint(path)
		if err != nil || checkpoint.Attempt != original || checkpoint.PID != child.pid() {
			return time.Time{}, fmt.Errorf("live generation checkpoint changed actor: %w", err)
		}
		return checkpoint.EnteredAt, nil
	}
	checkpoint, err := loadLiveStalePortCheckpoint(path)
	if err != nil || checkpoint.Attempt != original || checkpoint.PID != child.pid() {
		return time.Time{}, fmt.Errorf("live stale-port checkpoint changed actor: %w", err)
	}
	return checkpoint.EnteredAt, nil
}

func waitForLiveArtifact(
	ctx context.Context,
	path string,
	exits <-chan liveChildExit,
) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case exit := <-exits:
			return fmt.Errorf("live Resume child exited before checkpoint: %w", exit.err)
		case <-ticker.C:
			if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() &&
				info.Mode()&os.ModeSymlink == 0 && info.Size() > 0 {
				return nil
			} else if err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
}

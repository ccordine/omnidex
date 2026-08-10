package cognitiongauntlet

import (
	"context"
	"fmt"
	"syscall"
	"time"
)

func runKilledInferencePrefix(
	ctx context.Context,
	config OfflinePromotionConfig,
	database *offlinePromotionDatabase,
	host *offlinePromotionHost,
	paths OfflinePromotionPaths,
	executable string,
	executableSHA string,
	boundary uint32,
	temporary string,
	bundle PublicInferenceBundle,
) (PausedInferenceCheckpoint, int, time.Time, error) {
	checkpointPath := takeoverProcessPath(temporary, "before-checkpoint")
	control, err := checkpointInferenceControl(boundary, checkpointPath)
	if err != nil {
		return PausedInferenceCheckpoint{}, 0, time.Time{}, err
	}
	process := newInferenceProcessConfig(config, database, host, paths, executableSHA, "")
	process.Control = control
	processPath := takeoverProcessPath(temporary, "before-process")
	if err := writePrivateProcessFile(processPath, process, "takeover source process configuration"); err != nil {
		return PausedInferenceCheckpoint{}, 0, time.Time{}, err
	}
	if err := database.enableInference(ctx); err != nil {
		return PausedInferenceCheckpoint{}, 0, time.Time{}, err
	}
	child, err := startOfflineChild(ctx, executable, "infer", processPath, executableSHA)
	if err != nil {
		return PausedInferenceCheckpoint{}, 0, time.Time{}, err
	}
	defer child.signal(syscall.SIGKILL)
	episode, err := PublicVariantEpisodeRef(bundle.Authority)
	if err != nil {
		return PausedInferenceCheckpoint{}, 0, time.Time{}, err
	}
	checkpoint, err := waitForPausedInference(
		ctx, database.repository, episode.ID, database.attempt, boundary, checkpointPath, child,
	)
	if err != nil {
		return PausedInferenceCheckpoint{}, 0, time.Time{}, err
	}
	pid := child.pid()
	if err := child.signal(syscall.SIGKILL); err != nil {
		return PausedInferenceCheckpoint{}, 0, time.Time{}, fmt.Errorf("kill cognition inference source: %w", err)
	}
	if err := requireKilledChild(child); err != nil {
		return PausedInferenceCheckpoint{}, 0, time.Time{}, err
	}
	killedAt := time.Now().UTC()
	if err := database.revokeInference(context.Background()); err != nil {
		return PausedInferenceCheckpoint{}, 0, time.Time{}, err
	}
	if err := ensureAbsent(paths.Episode, "killed cognition episode seal"); err != nil {
		return PausedInferenceCheckpoint{}, 0, time.Time{}, err
	}
	return checkpoint, pid, killedAt, nil
}

func runReplacementInference(
	ctx context.Context,
	config OfflinePromotionConfig,
	database *offlinePromotionDatabase,
	host *offlinePromotionHost,
	paths OfflinePromotionPaths,
	executable string,
	executableSHA string,
	boundary uint32,
	temporary string,
	bundle PublicInferenceBundle,
	before PausedInferenceCheckpoint,
) (PausedInferenceCheckpoint, int, time.Time, error) {
	beforePath := takeoverProcessPath(temporary, "before-checkpoint")
	afterPath := takeoverProcessPath(temporary, "after-checkpoint")
	control, err := replacementInferenceControl(boundary, afterPath, beforePath)
	if err != nil {
		return PausedInferenceCheckpoint{}, 0, time.Time{}, err
	}
	process := newInferenceProcessConfig(config, database, host, paths, executableSHA, "")
	process.Control = control
	processPath := takeoverProcessPath(temporary, "replacement-process")
	if err := writePrivateProcessFile(processPath, process, "takeover replacement process configuration"); err != nil {
		return PausedInferenceCheckpoint{}, 0, time.Time{}, err
	}
	if err := database.enableInference(ctx); err != nil {
		return PausedInferenceCheckpoint{}, 0, time.Time{}, err
	}
	child, err := startOfflineChild(ctx, executable, "infer", processPath, executableSHA)
	if err != nil {
		return PausedInferenceCheckpoint{}, 0, time.Time{}, err
	}
	defer child.signal(syscall.SIGKILL)
	episode, err := PublicVariantEpisodeRef(bundle.Authority)
	if err != nil {
		return PausedInferenceCheckpoint{}, 0, time.Time{}, err
	}
	after, err := waitForPausedInference(
		ctx, database.repository, episode.ID, database.attempt, boundary, afterPath, child,
	)
	if err != nil {
		return PausedInferenceCheckpoint{}, 0, time.Time{}, err
	}
	if _, err := NewTakeoverContinuityProof(before.PreCall, after.PreCall); err != nil {
		return PausedInferenceCheckpoint{}, 0, time.Time{}, err
	}
	if before.Prefix != after.Prefix || before.SuccessfulActions != after.SuccessfulActions {
		return PausedInferenceCheckpoint{}, 0, time.Time{}, fmt.Errorf("replacement changed the sealed runtime prefix")
	}
	pid := child.pid()
	if err := child.signal(syscall.SIGCONT); err != nil {
		return PausedInferenceCheckpoint{}, 0, time.Time{}, fmt.Errorf("resume replacement cognition inference: %w", err)
	}
	if _, err := child.wait(); err != nil {
		return PausedInferenceCheckpoint{}, 0, time.Time{}, err
	}
	return after, pid, time.Now().UTC(), nil
}

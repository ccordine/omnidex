package cognitiongauntlet

import (
	"context"
	"fmt"
	"syscall"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

type takeoverInterruption struct {
	Boundary       inferenceBoundary
	Before         PausedInferenceCheckpoint
	After          PausedInferenceCheckpoint
	Original       model.StepAttemptAuthority
	Replacement    model.StepAttemptAuthority
	OriginalPID    int
	ReplacementPID int
	OriginalDied   time.Time
}

func runKilledChainedReplacement(
	ctx context.Context,
	config OfflinePromotionConfig,
	database *offlinePromotionDatabase,
	host *offlinePromotionHost,
	paths OfflinePromotionPaths,
	executable string,
	executableSHA string,
	resume PausedInferenceCheckpoint,
	next inferenceBoundary,
	temporary string,
	bundle PublicInferenceBundle,
	segment int,
) (PausedInferenceCheckpoint, PausedInferenceCheckpoint, int, time.Time, error) {
	if segment <= 0 || resume.Boundary.Validate() != nil || next.Validate() != nil {
		return PausedInferenceCheckpoint{}, PausedInferenceCheckpoint{}, 0, time.Time{},
			fmt.Errorf("chained takeover segment authority is invalid")
	}
	label := fmt.Sprintf("chain-%03d", segment)
	sourcePath := takeoverProcessPath(temporary, label+"-source")
	verificationPath := takeoverProcessPath(temporary, label+"-verification")
	nextPath := takeoverProcessPath(temporary, label+"-next")
	if err := SealPausedInferenceCheckpoint(sourcePath, resume); err != nil {
		return PausedInferenceCheckpoint{}, PausedInferenceCheckpoint{}, 0, time.Time{}, err
	}
	control, err := newChainedReplacementInferenceControl(
		next, nextPath, sourcePath, verificationPath, resume.Boundary,
	)
	if err != nil {
		return PausedInferenceCheckpoint{}, PausedInferenceCheckpoint{}, 0, time.Time{}, err
	}
	process := newInferenceProcessConfig(config, database, host, paths, executableSHA, "")
	process.Control = control
	processPath := takeoverProcessPath(temporary, label+"-process")
	if err := writePrivateProcessFile(
		processPath, process, "chained takeover process configuration",
	); err != nil {
		return PausedInferenceCheckpoint{}, PausedInferenceCheckpoint{}, 0, time.Time{}, err
	}
	if err := database.enableInference(ctx); err != nil {
		return PausedInferenceCheckpoint{}, PausedInferenceCheckpoint{}, 0, time.Time{}, err
	}
	child, err := startOfflineChild(ctx, executable, "infer", processPath, executableSHA)
	if err != nil {
		return PausedInferenceCheckpoint{}, PausedInferenceCheckpoint{}, 0, time.Time{}, err
	}
	defer child.signal(syscall.SIGKILL)
	episode, err := PublicVariantEpisodeRef(bundle.Authority)
	if err != nil {
		return PausedInferenceCheckpoint{}, PausedInferenceCheckpoint{}, 0, time.Time{}, err
	}
	verification, err := waitForPausedInference(
		ctx, database.pool, database.repository, episode.ID, database.attempt,
		resume.Boundary, verificationPath, child,
	)
	if err != nil {
		return PausedInferenceCheckpoint{}, PausedInferenceCheckpoint{}, 0, time.Time{}, err
	}
	if _, err := NewTakeoverContinuityProof(resume.PreCall, verification.PreCall); err != nil {
		return PausedInferenceCheckpoint{}, PausedInferenceCheckpoint{}, 0, time.Time{},
			err
	}
	if resume.Prefix != verification.Prefix {
		return PausedInferenceCheckpoint{}, PausedInferenceCheckpoint{}, 0, time.Time{},
			fmt.Errorf("chained takeover changed its sealed runtime prefix")
	}
	if err := child.signal(syscall.SIGCONT); err != nil {
		return PausedInferenceCheckpoint{}, PausedInferenceCheckpoint{}, 0, time.Time{}, err
	}
	stopped, err := waitForPausedInference(
		ctx, database.pool, database.repository, episode.ID, database.attempt,
		next, nextPath, child,
	)
	if err != nil {
		return PausedInferenceCheckpoint{}, PausedInferenceCheckpoint{}, 0, time.Time{}, err
	}
	pid := child.pid()
	if err := child.signal(syscall.SIGKILL); err != nil {
		return PausedInferenceCheckpoint{}, PausedInferenceCheckpoint{}, 0, time.Time{}, err
	}
	if err := requireKilledChild(child); err != nil {
		return PausedInferenceCheckpoint{}, PausedInferenceCheckpoint{}, 0, time.Time{}, err
	}
	killedAt := time.Now().UTC()
	if err := database.revokeInference(context.Background()); err != nil {
		return PausedInferenceCheckpoint{}, PausedInferenceCheckpoint{}, 0, time.Time{}, err
	}
	if err := ensureAbsent(paths.Episode, "interrupted cognition episode seal"); err != nil {
		return PausedInferenceCheckpoint{}, PausedInferenceCheckpoint{}, 0, time.Time{}, err
	}
	return verification, stopped, pid, killedAt, nil
}

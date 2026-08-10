package cognitiongauntlet

import (
	"context"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func captureLivePreCall(
	ctx context.Context,
	setup *offlineResumeExecution,
	authority model.StepAttemptAuthority,
) (SemanticPreCallCheckpoint, error) {
	episode, err := PublicVariantEpisodeRef(setup.bundle.Authority)
	if err != nil {
		return SemanticPreCallCheckpoint{}, err
	}
	return CaptureSemanticPreCallCheckpoint(
		ctx, setup.database.repository, episode.ID, authority,
	)
}

func runLiveReplacement(
	ctx context.Context,
	setup *offlineResumeExecution,
	port liveStalePort,
	processPath string,
) (int, time.Time, error) {
	control, err := liveStalePortRecoveryControl(port)
	if err != nil {
		return 0, time.Time{}, err
	}
	process := newInferenceProcessConfig(
		setup.config, setup.database, setup.host, setup.paths, setup.executableSHA256, "",
	)
	process.Control = control
	if err := writePrivateProcessFile(
		processPath, process, "live Resume replacement process",
	); err != nil {
		return 0, time.Time{}, err
	}
	if err := setup.database.enableInference(ctx); err != nil {
		return 0, time.Time{}, err
	}
	pid, err := runOfflineChild(
		ctx, setup.executable, "infer", processPath, setup.executableSHA256,
	)
	exitedAt := time.Now().UTC()
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("live Resume replacement for %q: %w", port, err)
	}
	return pid, exitedAt, nil
}

func waitForExpectedLiveRejection(
	ctx context.Context,
	exits <-chan liveChildExit,
) (liveChildExit, error) {
	select {
	case <-ctx.Done():
		return liveChildExit{}, ctx.Err()
	case exit := <-exits:
		if exit.pid <= 0 || exit.at.IsZero() || exit.err == nil {
			return liveChildExit{}, fmt.Errorf("expired live Resume actor did not exit loudly")
		}
		return exit, nil
	}
}

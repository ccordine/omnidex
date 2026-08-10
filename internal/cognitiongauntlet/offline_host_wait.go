package cognitiongauntlet

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

func waitForOfflineHostReady(
	ctx context.Context,
	config hostProcessConfig,
	expectedPID int,
	exits <-chan offlineHostExit,
) (hostProcessReady, error) {
	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case exit := <-exits:
			return hostProcessReady{}, fmt.Errorf(
				"offline host exited before readiness: %w", exit.err,
			)
		case <-waitCtx.Done():
			return hostProcessReady{}, fmt.Errorf("wait for offline host readiness: %w", waitCtx.Err())
		case <-ticker.C:
			ready, err := loadHostProcessReady(config.ReadyPath, config)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return hostProcessReady{}, err
			}
			if ready.PID != expectedPID {
				return hostProcessReady{}, fmt.Errorf("offline host readiness PID differs from child")
			}
			return ready, nil
		}
	}
}

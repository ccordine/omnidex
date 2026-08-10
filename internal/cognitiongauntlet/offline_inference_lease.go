package cognitiongauntlet

import (
	"context"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func maintainInferenceLease(
	ctx context.Context,
	repository *queue.Repository,
	attempt model.StepAttemptAuthority,
	cancel context.CancelCauseFunc,
) <-chan error {
	done := make(chan error, 1)
	go func() {
		defer close(done)
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				done <- nil
				return
			case <-ticker.C:
				if _, err := repository.RenewStepAttempt(ctx, attempt); err != nil {
					failure := fmt.Errorf("renew offline inference attempt: %w", err)
					cancel(failure)
					done <- failure
					return
				}
			}
		}
	}()
	return done
}

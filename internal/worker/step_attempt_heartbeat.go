package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

const stepAttemptRenewInterval = 25 * time.Second

func (s *Service) watchStepAttemptLease(
	ctx context.Context,
	authority model.StepAttemptAuthority,
) (context.Context, func() error) {
	leaseCtx, cancel := context.WithCancel(ctx)
	result := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(stepAttemptRenewInterval)
		defer ticker.Stop()
		for {
			select {
			case <-leaseCtx.Done():
				result <- nil
				return
			case <-ticker.C:
				if _, err := s.repo.RenewStepAttempt(leaseCtx, authority); err != nil {
					result <- fmt.Errorf("renew exact step attempt: %w", err)
					cancel()
					return
				}
			}
		}
	}()
	return leaseCtx, func() error {
		cancel()
		return <-result
	}
}

func (s *Service) finishStepAttemptWatch(
	workErr, leaseErr error,
) error {
	if workErr == nil {
		if leaseErr != nil {
			s.logf("step operation completed before lease bookkeeping failed: %v", leaseErr)
		}
		return nil
	}
	if leaseErr != nil {
		return errors.Join(workErr, leaseErr)
	}
	return workErr
}

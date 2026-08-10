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
	ctx context.Context,
	claim *model.ClaimedStep,
	workErr, leaseErr error,
) error {
	if leaseErr == nil {
		return workErr
	}
	if workErr != nil {
		return errors.Join(workErr, leaseErr)
	}
	jobStatus, stepStatus, stateErr := s.repo.GetStepRuntimeState(ctx, claim.Job.ID, claim.Step.ID)
	if stateErr != nil {
		return errors.Join(leaseErr, fmt.Errorf("read state after lease renewal failure: %w", stateErr))
	}
	if terminalWorkerStepStatus(stepStatus) || jobStatus == model.JobStatusCanceled {
		return nil
	}
	return leaseErr
}

func terminalWorkerStepStatus(status string) bool {
	switch status {
	case model.StepStatusCompleted, model.StepStatusFailed,
		model.StepStatusWaiting, model.StepStatusCanceled:
		return true
	default:
		return false
	}
}

package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func (s *Service) Start(ctx context.Context) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("worker start requires repository authority")
	}
	pollInterval, err := time.ParseDuration(s.pollInterval)
	if err != nil {
		return fmt.Errorf("WORKER_POLL_INTERVAL must be a duration: %w", err)
	}
	if pollInterval <= 0 {
		return fmt.Errorf("WORKER_POLL_INTERVAL must be positive, received %s", pollInterval)
	}
	s.run(ctx, "serial-worker", pollInterval)
	return nil
}

func (s *Service) run(ctx context.Context, workerID string, pollInterval time.Duration) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		claim, err := s.repo.ClaimNextStep(ctx, workerID)
		if err != nil {
			s.logf("worker=%s claim error: %v", workerID, err)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			continue
		}
		if claim == nil {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			continue
		}
		s.emitStepEvent(claim.Authority, "step_start", fmt.Sprintf("action=%s worker=%s", claim.Step.Action, workerID))
		if err := s.processStep(ctx, claim); err != nil {
			if s.skipFailureForLostExecutionAuthority(workerID, claim, err) {
				continue
			}
			if s.skipFailureForControlledCancel(ctx, workerID, claim, err) {
				continue
			}
			s.emitStepEvent(claim.Authority, "step_error", err.Error())
			s.logf("worker=%s job=%d step=%d action=%s failed: %v", workerID, claim.Job.ID, claim.Step.ID, claim.Step.Action, err)
			failCommand, identityErr := failClaimedStepCommand(claim, err.Error())
			if identityErr != nil {
				s.logf("worker=%s job=%d step=%d failure identity error: %v", workerID, claim.Job.ID, claim.Step.ID, identityErr)
				continue
			}
			failErr := s.repo.FailStep(ctx, failCommand)
			if failErr != nil {
				s.logf("worker=%s job=%d step=%d fail update error: %v", workerID, claim.Job.ID, claim.Step.ID, failErr)
			}
			continue
		}
		s.emitStepEvent(claim.Authority, "step_complete", fmt.Sprintf("action=%s worker=%s", claim.Step.Action, workerID))
		s.logf(
			"worker=%s job=%d step=%d action=%s completed",
			workerID, claim.Job.ID, claim.Step.ID, claim.Step.Action,
		)
	}
}

func (s *Service) processStep(ctx context.Context, claim *model.ClaimedStep) error {
	controlCtx, stopControl := s.watchStepControl(ctx, claim.Job.ID, claim.Step.ID)
	stepCtx, stopLease := s.watchStepAttemptLease(
		controlCtx, claim.Authority, claim.LeaseDeadline,
	)
	workErr := s.runNativeV3Step(stepCtx, claim, claim.Step.Action)
	leaseErr := stopLease()
	controlErr := stopControl()
	return s.finishStepAttemptWatch(workErr, errors.Join(leaseErr, controlErr))
}

func (s *Service) watchStepControl(ctx context.Context, jobID, stepID int64) (context.Context, func() error) {
	stepCtx, cancel := context.WithCancel(ctx)
	result := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(stepControlPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stepCtx.Done():
				result <- nil
				return
			case <-ticker.C:
			}
			jobStatus, stepStatus, err := s.repo.GetStepRuntimeState(stepCtx, jobID, stepID)
			if err != nil {
				if errors.Is(err, context.Canceled) || stepCtx.Err() != nil {
					result <- nil
					return
				}
				result <- fmt.Errorf(
					"%w: read job %d step %d runtime authority: %v",
					errStepExecutionAuthorityLost, jobID, stepID, err,
				)
				cancel()
				return
			}
			if jobStatus == model.JobStatusCanceled || stepStatus == model.StepStatusCanceled {
				cancel()
				result <- nil
				return
			}
		}
	}()
	return stepCtx, func() error {
		cancel()
		return <-result
	}
}

func (s *Service) skipFailureForLostExecutionAuthority(
	workerID string,
	claim *model.ClaimedStep,
	err error,
) bool {
	if !errors.Is(err, errStepExecutionAuthorityLost) &&
		!errors.Is(err, queue.ErrStaleStepAttempt) {
		return false
	}
	s.emitStepEvent(claim.Authority, "step_authority_lost", err.Error())
	s.logf(
		"worker=%s job=%d step=%d relinquished stale execution authority: %v",
		workerID, claim.Job.ID, claim.Step.ID, err,
	)
	return true
}

func (s *Service) skipFailureForControlledCancel(ctx context.Context, workerID string, claim *model.ClaimedStep, err error) bool {
	if !errors.Is(err, context.Canceled) {
		return false
	}
	if ctx.Err() != nil {
		return true
	}
	jobStatus, stepStatus, stateErr := s.repo.GetStepRuntimeState(ctx, claim.Job.ID, claim.Step.ID)
	if stateErr != nil {
		s.logf("worker=%s job=%d step=%d cancel-state lookup error: %v", workerID, claim.Job.ID, claim.Step.ID, stateErr)
		return false
	}
	if jobStatus == model.JobStatusCanceled || stepStatus == model.StepStatusCanceled {
		s.logf("worker=%s job=%d step=%d action=%s canceled", workerID, claim.Job.ID, claim.Step.ID, claim.Step.Action)
		s.emitStepEvent(claim.Authority, "step_canceled", fmt.Sprintf("action=%s worker=%s", claim.Step.Action, workerID))
		return true
	}
	return false
}

package worker

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func (s *Service) Start(ctx context.Context) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("worker start requires repository authority")
	}
	workerCount, err := strconv.Atoi(s.workerCount)
	if err != nil {
		return fmt.Errorf("WORKER_COUNT must be an integer: %w", err)
	}
	if workerCount < 1 {
		return fmt.Errorf("WORKER_COUNT must be at least 1, received %d", workerCount)
	}
	pollInterval, err := time.ParseDuration(s.pollInterval)
	if err != nil {
		return fmt.Errorf("WORKER_POLL_INTERVAL must be a duration: %w", err)
	}
	if pollInterval <= 0 {
		return fmt.Errorf("WORKER_POLL_INTERVAL must be positive, received %s", pollInterval)
	}
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		workerID := fmt.Sprintf("worker-%d", i+1)
		go func(id string) {
			defer wg.Done()
			s.run(ctx, id, pollInterval)
		}(workerID)
	}
	<-ctx.Done()
	wg.Wait()
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
	defer stopControl()
	stepCtx, stopLease := s.watchStepAttemptLease(controlCtx, claim.Authority)
	workErr := s.runNativeV3Step(stepCtx, claim, claim.Step.Action)
	leaseErr := stopLease()
	return s.finishStepAttemptWatch(workErr, leaseErr)
}

func (s *Service) watchStepControl(ctx context.Context, jobID, stepID int64) (context.Context, func()) {
	stepCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(stepControlPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stepCtx.Done():
				return
			case <-ticker.C:
			}
			jobStatus, stepStatus, err := s.repo.GetStepRuntimeState(stepCtx, jobID, stepID)
			if err != nil {
				if errors.Is(err, context.Canceled) || stepCtx.Err() != nil {
					return
				}
				s.logf("job=%d step=%d control poll error: %v", jobID, stepID, err)
				continue
			}
			if jobStatus == model.JobStatusCanceled || stepStatus == model.StepStatusCanceled {
				cancel()
				return
			}
		}
	}()
	return stepCtx, func() {
		cancel()
		<-done
	}
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

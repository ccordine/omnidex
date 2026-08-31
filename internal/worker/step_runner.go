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
	if s == nil || s.repo == nil || s.failStep == nil {
		return fmt.Errorf("worker start requires repository authority")
	}
	pollInterval, err := time.ParseDuration(s.pollInterval)
	if err != nil {
		return fmt.Errorf("WORKER_POLL_INTERVAL must be a duration: %w", err)
	}
	if pollInterval <= 0 {
		return fmt.Errorf("WORKER_POLL_INTERVAL must be positive, received %s", pollInterval)
	}
	return s.run(ctx, "serial-worker", pollInterval)
}

func (s *Service) run(ctx context.Context, workerID string, pollInterval time.Duration) error {
	return s.runLoop(ctx, workerID, pollInterval, s.repo.ClaimNextStep, s.runClaim)
}

type claimNextStepFunc func(context.Context, string) (*model.ClaimedStep, error)
type runClaimFunc func(context.Context, string, *model.ClaimedStep) error

func (s *Service) runLoop(
	ctx context.Context,
	workerID string,
	pollInterval time.Duration,
	claimNextStep claimNextStepFunc,
	runClaim runClaimFunc,
) error {
	if claimNextStep == nil || runClaim == nil {
		return fmt.Errorf("worker loop requires claim and execution authority")
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		claim, err := claimNextStep(ctx, workerID)
		if err != nil {
			s.logf("worker=%s claim error: %v", workerID, err)
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
			}
			continue
		}
		if claim == nil {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
			}
			continue
		}
		releaseRuntimeChannel := s.bindRuntimeEventChannel(claim.Job)
		claimErr := runClaim(ctx, workerID, claim)
		releaseRuntimeChannel()
		if claimErr != nil {
			// Continuing would leave an uncommitted running attempt eligible for
			// lease reclamation and could repeat the same inference indefinitely.
			return fmt.Errorf(
				"worker %q cannot persist terminal state for job %d step %d: %w",
				workerID, claim.Job.ID, claim.Step.ID, claimErr,
			)
		}
	}
}

func (s *Service) runClaim(ctx context.Context, workerID string, claim *model.ClaimedStep) error {
	s.emitStepEvent(claim.Authority, "step_start", fmt.Sprintf("action=%s worker=%s", claim.Step.Action, workerID))
	if err := s.processStep(ctx, claim); err != nil {
		if s.skipFailureForLostExecutionAuthority(workerID, claim, err) {
			return nil
		}
		if s.skipFailureForControlledCancel(ctx, workerID, claim, err) {
			return nil
		}
		s.emitStepEvent(claim.Authority, "step_error", err.Error())
		s.logf("worker=%s job=%d step=%d action=%s failed: %v", workerID, claim.Job.ID, claim.Step.ID, claim.Step.Action, err)
		failCommand, identityErr := failClaimedStepCommand(claim, err.Error())
		if identityErr != nil {
			return fmt.Errorf("construct exact failure lifecycle operation: %w", identityErr)
		}
		failErr := s.failStep(ctx, failCommand)
		if failErr != nil {
			if errors.Is(failErr, queue.ErrStaleStepAttempt) {
				s.emitStepEvent(claim.Authority, "step_authority_lost", failErr.Error())
				s.logf(
					"worker=%s job=%d step=%d failure commit relinquished stale execution authority: %v",
					workerID, claim.Job.ID, claim.Step.ID, failErr,
				)
				return nil
			}
			s.emitStepEvent(claim.Authority, "step_failure_persistence_failed", failErr.Error())
			return fmt.Errorf("commit failed step lifecycle operation: %w", failErr)
		}
		s.emitStepEvent(
			claim.Authority,
			"step_failed",
			fmt.Sprintf("action=%s worker=%s", claim.Step.Action, workerID),
		)
		return nil
	}
	s.emitStepEvent(claim.Authority, "step_complete", fmt.Sprintf("action=%s worker=%s", claim.Step.Action, workerID))
	s.logf(
		"worker=%s job=%d step=%d action=%s completed",
		workerID, claim.Job.ID, claim.Step.ID, claim.Step.Action,
	)
	return nil
}

func (s *Service) processStep(ctx context.Context, claim *model.ClaimedStep) error {
	controlCtx, stopControl := s.watchStepControl(ctx, claim.Authority)
	stepCtx, stopLease := s.watchStepAttemptLease(
		controlCtx, claim.Authority, claim.LeaseDeadline,
	)
	workErr := s.runNativeV3Step(stepCtx, claim, claim.Step.Action)
	leaseErr := stopLease()
	controlErr := stopControl()
	return s.finishStepAttemptWatch(workErr, errors.Join(leaseErr, controlErr))
}

func (s *Service) watchStepControl(
	ctx context.Context,
	authority model.StepAttemptAuthority,
) (context.Context, func() error) {
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
			if err := s.repo.RequireActiveStepAttempt(stepCtx, authority); err != nil {
				if errors.Is(err, context.Canceled) || stepCtx.Err() != nil {
					result <- nil
					return
				}
				result <- fmt.Errorf(
					"%w: job %d generation %d step %d attempt %d changed: %v",
					errStepExecutionAuthorityLost,
					authority.JobID,
					authority.Generation,
					authority.StepID,
					authority.Attempt,
					err,
				)
				cancel()
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
		!errors.Is(err, queue.ErrStaleStepAttempt) &&
		!errors.Is(err, queue.ErrLLMCallTerminalizedByAttempt) {
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

package worker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/specialist"
)

func (s *Service) Start(ctx context.Context) error {
	if err := s.refreshSkillRegistry(ctx); err != nil {
		return fmt.Errorf("initialize authoritative worker skill registry: %w", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < s.workerCount; i++ {
		wg.Add(1)
		workerID := fmt.Sprintf("worker-%d", i+1)
		go func(id string) {
			defer wg.Done()
			s.run(ctx, id)
		}(workerID)
	}
	<-ctx.Done()
	wg.Wait()
	return nil
}

func (s *Service) run(ctx context.Context, workerID string) {
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		claim, err := s.repo.ClaimNextStep(ctx, workerID)
		if err != nil {
			s.logger.Printf("worker=%s claim error: %v", workerID, err)
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
		phase := pipelinePhaseForAction(claim.Step.Action)
		stepRole := specialist.ForPipelineAction(claim.Step.Action)
		s.emitStepContext(claim.Authority, "phase", phase)
		s.emitStepContext(claim.Authority, "specialist_role", strings.Join(specialist.DetailLines(stepRole), "\n"))
		s.emitStepEvent(claim.Authority, "step_start", fmt.Sprintf("phase=%s action=%s worker=%s specialist=%s", phase, claim.Step.Action, workerID, strings.TrimSpace(stepRole.ID)))
		if err := s.processStep(ctx, claim); err != nil {
			if s.skipFailureForControlledCancel(ctx, workerID, claim, err) {
				continue
			}
			s.emitStepEvent(claim.Authority, "step_error", err.Error())
			s.logger.Printf("worker=%s job=%d step=%d action=%s failed: %v", workerID, claim.Job.ID, claim.Step.ID, claim.Step.Action, err)
			failCommand, identityErr := failClaimedStepCommand(claim, err.Error())
			if identityErr != nil {
				s.logger.Printf("worker=%s job=%d step=%d failure identity error: %v", workerID, claim.Job.ID, claim.Step.ID, identityErr)
				continue
			}
			failErr := s.repo.FailStep(ctx, failCommand)
			if failErr != nil {
				s.logger.Printf("worker=%s job=%d step=%d fail update error: %v", workerID, claim.Job.ID, claim.Step.ID, failErr)
			} else {
				s.notifyJobFinishedForJob(ctx, claim.Job.ID)
			}
			continue
		}
		s.emitStepEvent(claim.Authority, "step_complete", fmt.Sprintf("action=%s worker=%s", claim.Step.Action, workerID))
	}
}

func (s *Service) processStep(ctx context.Context, claim *model.ClaimedStep) error {
	if _, err := modelRoutingFromJobMetadata(claim.Job.Metadata, s.models); err != nil {
		return err
	}
	action := strings.ToLower(strings.TrimSpace(claim.Step.Action))
	contexts := contextsToMap(claim.Contexts)
	controlCtx, stopControl := s.watchStepControl(ctx, claim.Job.ID, claim.Step.ID)
	defer stopControl()
	stepCtx, stopLease := s.watchStepAttemptLease(controlCtx, claim.Authority)
	workErr := s.processClaimedAction(stepCtx, claim, contexts, action)
	leaseErr := stopLease()
	return s.finishStepAttemptWatch(ctx, claim, workErr, leaseErr)
}

func (s *Service) processClaimedAction(stepCtx context.Context, claim *model.ClaimedStep, contexts map[string]string, action string) error {
	if strings.HasPrefix(action, "v3_") {
		if s.nativeV3Runner != nil {
			return s.nativeV3Runner(stepCtx, claim, contexts, action)
		}
		return s.runNativeV3Step(stepCtx, claim, contexts, action)
	}
	switch action {
	case "external_agent_execute":
		return s.runExternalAgentStep(stepCtx, claim, contexts)
	case "data_source_query":
		return s.runDataSourceQueryStep(stepCtx, claim)
	case "data_source_explore":
		return s.runDataSourceExploreStep(stepCtx, claim)
	case "project_debugger":
		return s.runProjectDebuggerStep(stepCtx, claim)
	case "scrum_card_llm":
		return s.runScrumCardLLMStep(stepCtx, claim)
	default:
		return fmt.Errorf("unsupported worker action %q", action)
	}
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
				s.logger.Printf("job=%d step=%d control poll error: %v", jobID, stepID, err)
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
		s.logger.Printf("worker=%s job=%d step=%d cancel-state lookup error: %v", workerID, claim.Job.ID, claim.Step.ID, stateErr)
		return false
	}
	if jobStatus == model.JobStatusCanceled || stepStatus == model.StepStatusCanceled {
		s.logger.Printf("worker=%s job=%d step=%d action=%s canceled", workerID, claim.Job.ID, claim.Step.ID, claim.Step.Action)
		s.emitStepEvent(claim.Authority, "step_canceled", fmt.Sprintf("action=%s worker=%s", claim.Step.Action, workerID))
		return true
	}
	return false
}

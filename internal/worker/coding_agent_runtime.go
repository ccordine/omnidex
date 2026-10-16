package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
)

func (runtime *nativeRuntimeV3) runAgentCodingAction(configuration codingagent.Configuration) error {
	if runtime.svc.agentProviders == nil {
		return fmt.Errorf("Agents mode has no registered execution environment")
	}
	scope, err := workspaceAuthorityForV3Job(runtime.claim.Job)
	if err != nil {
		return err
	}
	identity, err := projectroot.DirectoryIdentity(scope.Root)
	if err != nil {
		return err
	}
	agentContext, cancelAgent, err := runtime.svc.nativeAgentWorkspaceContext(runtime.ctx, runtime.claim.Job, scope, identity)
	if err != nil {
		return err
	}
	defer cancelAgent()
	if err := runtime.acquireWorkspaceMutationFence(scope.Root); err != nil {
		return err
	}
	if err := runtime.workspaceFence.Reattest(scope.Root); err != nil {
		return fmt.Errorf("native agent environment must own the exact selected workspace: %w", err)
	}
	providers, verifier, err := runtime.svc.agentProviders()
	if err != nil {
		return err
	}
	configuration, err = runtime.svc.repo.FrozenCodingLedgerConfiguration(runtime.ctx, runtime.claim.Authority)
	if err != nil {
		return err
	}
	controller := codingagent.Controller{JobID: runtime.claim.Job.ID, Generation: runtime.claim.Authority.Generation,
		Root: scope.Root, Configuration: configuration, Providers: providers, Verifier: verifier,
		Environment:   &codingAgentEnvironment{runtime: runtime, root: scope.Root, identity: identity},
		Evidence:      runtime.svc.repo.CodingAgentEvidence(runtime.claim.Authority),
		NativeInput:   runtime.waitForNativeCodingInput,
		MinifyContext: runtime.minifyCodingContext,
		Activity: func(activity model.AgentActivity) {
			runtime.svc.emitRuntimeEvent(RuntimeEvent{
				JobID: runtime.claim.Authority.JobID, StepID: runtime.claim.Authority.StepID,
				Attempt: runtime.claim.Authority.Attempt, Kind: "agent_activity", AgentActivity: &activity,
			})
		},
		CallActivity: func(activity model.AgentCallActivity) {
			runtime.svc.emitRuntimeEvent(RuntimeEvent{
				JobID: runtime.claim.Authority.JobID, StepID: runtime.claim.Authority.StepID,
				Attempt: runtime.claim.Authority.Attempt, Kind: "agent_call_activity", AgentCallActivity: &activity,
			})
		}}
	controller.CompletedTaskIDs, err = runtime.svc.repo.CodingAcceptedTaskIDs(agentContext, runtime.claim.Job.ID)
	if err != nil {
		return err
	}
	stopHeartbeat := runtime.svc.startProgressHeartbeat(runtime.ctx, runtime.claim.Authority, "scoped-agent-work")
	defer stopHeartbeat()
	for {
		task, err := runtime.svc.repo.NextIncompleteCodingLedgerTask(agentContext, runtime.claim.Authority)
		if err != nil {
			return err
		}
		if task == nil {
			break
		}
		if err := controller.RunTask(agentContext, *task); err != nil {
			var incomplete *codingagent.IncompleteTaskError
			if errors.As(err, &incomplete) {
				current, exists, err := runtime.svc.repo.CurrentCodingLedgerForJob(agentContext, runtime.claim.Job.ID)
				if err != nil {
					return err
				}
				if !exists {
					return fmt.Errorf("incomplete task lost its authoritative ledger")
				}
				ledger, err := readCompleteCodingLedger(agentContext, runtime.svc.repo, current.LedgerID)
				if err != nil {
					return err
				}
				if err := controller.PlanContinuation(agentContext, *task, incomplete, ledger.Ledger); err != nil {
					return err
				}
				if err := runtime.svc.repo.WaitForCodingExecutionInput(agentContext, runtime.claim.Authority, task.ID); err != nil {
					return err
				}
				return errCodingExecutionInputPending
			}
			var input *codingagent.ExecutionInputRequired
			if errors.As(err, &input) {
				if err := runtime.svc.repo.WaitForCodingExecutionInput(agentContext, runtime.claim.Authority, input.TaskID); err != nil {
					return err
				}
				return errCodingExecutionInputPending
			}
			return err
		}
		if err := runtime.svc.repo.AcceptCodingLedgerTask(agentContext, runtime.claim.Authority, task.ID); err != nil {
			return err
		}
		controller.CompletedTaskIDs = append(controller.CompletedTaskIDs, task.ID)
	}
	if err := agentContext.Err(); err != nil {
		return err
	}
	if err := runtime.workspaceFence.Reattest(scope.Root); err != nil {
		return err
	}
	entries, err := controller.Evidence.Read(agentContext)
	if err != nil {
		return err
	}
	taskCount, checkCount, err := runtime.svc.repo.CodingLedgerCompletionCounts(agentContext, runtime.claim.Authority)
	if err != nil {
		return err
	}
	recap, err := codingagent.TaskLedgerCompletionRecap(entries, taskCount, checkCount, model.MaxChannelContentBytes)
	if err != nil {
		return err
	}
	return runtime.completeAppliedWorkspace(recap)
}

func (runtime *nativeRuntimeV3) waitForNativeCodingInput(ctx context.Context, request codingagent.NativeInputRequest, callID int64, taskID string) (json.RawMessage, error) {
	id, err := runtime.svc.repo.WaitForCodingNativeInput(ctx, runtime.claim.Authority, request, callID, taskID)
	if err != nil {
		return nil, err
	}
	runtime.svc.emitStepEvent(runtime.claim.Authority, "step_waiting_input", request.Method)
	// The existing attempt lease and control watcher remain live. Disconnecting
	// a UI does not close this native process; cancellation still does.
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
		response, found, err := runtime.svc.repo.CodingNativeInputReply(ctx, runtime.claim.Authority, id)
		if err != nil {
			return nil, err
		}
		if found {
			if err := runtime.svc.repo.RequireActiveStepAttempt(ctx, runtime.claim.Authority); err != nil {
				return nil, err
			}
			runtime.svc.emitStepEvent(runtime.claim.Authority, "step_running", "native owner input received")
			return response, nil
		}
	}
}

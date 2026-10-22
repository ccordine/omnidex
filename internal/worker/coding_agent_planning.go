package worker

import (
	"context"
	"errors"
	"fmt"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/queue"
)

func (runtime *nativeRuntimeV3) runAgentPlanningAction(configuration codingagent.Configuration) error {
	if runtime.svc.agentProviders == nil {
		return fmt.Errorf("Agents mode has no registered planning environment")
	}
	scope, err := runtime.svc.workspaceScopeForV3Job(runtime.ctx, runtime.claim.Job)
	if err != nil {
		return err
	}
	if err := runtime.acquireWorkspaceMutationFence(scope.Root); err != nil {
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
	request, err := runtime.codingLedgerRequest()
	if err != nil {
		return err
	}
	providers, err := runtime.svc.agentProviders()
	if err != nil {
		return err
	}
	controller := codingagent.Controller{
		JobID: runtime.claim.Job.ID, Generation: runtime.claim.Authority.Generation,
		Root: scope.Root, Configuration: configuration,
		Providers:   providers,
		Environment: &codingAgentEnvironment{runtime: runtime, root: scope.Root, identity: identity},
		Evidence:    runtime.svc.repo.CodingAgentEvidence(runtime.claim.Authority),
		NativeInput: runtime.waitForNativeCodingInput,
		Activity: func(activity model.AgentActivity) {
			runtime.svc.emitRuntimeEvent(RuntimeEvent{JobID: runtime.claim.Authority.JobID, StepID: runtime.claim.Authority.StepID, Attempt: runtime.claim.Authority.Attempt, Kind: "agent_activity", AgentActivity: &activity})
		},
		CallActivity: func(activity model.AgentCallActivity) {
			runtime.svc.emitRuntimeEvent(RuntimeEvent{JobID: runtime.claim.Authority.JobID, StepID: runtime.claim.Authority.StepID, Attempt: runtime.claim.Authority.Attempt, Kind: "agent_call_activity", AgentCallActivity: &activity})
		},
	}
	controller.CompletedTaskIDs, err = runtime.svc.repo.CodingAcceptedTaskIDs(agentContext, runtime.claim.Job.ID)
	if err != nil {
		return err
	}
	stopHeartbeat := runtime.svc.startProgressHeartbeat(runtime.ctx, runtime.claim.Authority, "scoped-agent-planning")
	defer stopHeartbeat()
	existing, exists, err := runtime.svc.repo.CurrentCodingLedgerForJob(runtime.ctx, runtime.claim.Job.ID)
	if err != nil {
		return err
	}
	turn := codingagent.PlanningTurn{
		OperationID: fmt.Sprintf("planning:%d:%d", runtime.claim.Job.ID, runtime.claim.Authority.Generation),
		OwnerTurnID: fmt.Sprintf("owner:%d:%d", runtime.claim.Job.ID, runtime.claim.Authority.Generation),
		Text:        request.planningText(exists), ObservedRevision: request.ObservedLedgerRevision,
	}
	if request.PlanningOperationID != "" {
		turn.OperationID = request.PlanningOperationID
	}
	alreadyApplied := false
	if exists {
		_, alreadyApplied, err = runtime.svc.repo.CodingLedgerRevisionForOperation(runtime.ctx, existing.LedgerID, turn.OperationID)
		if err != nil {
			return err
		}
	}
	if exists && turn.ObservedRevision == 0 && !alreadyApplied {
		return fmt.Errorf("planning turn %q lacks the exact ledger revision observed by the client", turn.OperationID)
	}
	var ledger queue.CodingLedgerRevision
	if exists && alreadyApplied {
		ledger, err = readCompleteCodingLedger(agentContext, runtime.svc.repo, existing.LedgerID)
	} else if exists {
		ledger, err = applyCodingLedgerPlanningTransaction(agentContext, runtime.svc.repo, &controller, existing.LedgerID, turn, nil)
	} else {
		ledger, err = createCodingLedgerPlanningTransaction(agentContext, runtime.svc.repo, &controller, runtime.claim.Job.ID, turn)
	}
	if err != nil {
		return runtime.planningInputError(agentContext, err)
	}
	if err := controller.PublishPlanningLedger(agentContext, turn, ledger.Ledger); err != nil {
		return err
	}
	ledger, err = runtime.finishPlanningConversation(agentContext, &controller, ledger)
	if err != nil {
		return runtime.planningInputError(agentContext, err)
	}
	runtime.svc.emitStepEvent(runtime.claim.Authority, "coding_plan_review_ready", fmt.Sprintf("ledger=%d revision=%d tasks=%d", ledger.LedgerID, ledger.Revision, len(ledger.Ledger.Tasks)))
	return errCodingLedgerReviewPending
}

func (runtime *nativeRuntimeV3) planningInputError(ctx context.Context, err error) error {
	var input *codingagent.ExecutionInputRequired
	if errors.As(err, &input) {
		if err := runtime.svc.repo.WaitForCodingExecutionInput(ctx, runtime.claim.Authority, ""); err != nil {
			return err
		}
		return errCodingExecutionInputPending
	}
	return err
}

type codingLedgerPlanningRepository interface {
	CreateCodingLedger(context.Context, queue.CreateCodingLedgerCommand) (queue.CodingLedgerRevision, error)
	ReadCodingLedger(context.Context, int64, queue.CodingLedgerReadOptions) (queue.CodingLedgerRevision, error)
	ApplyCodingLedgerMutation(context.Context, queue.ApplyCodingLedgerMutationCommand) (queue.CodingLedgerRevision, error)
	ProposeUnchangedCodingLedger(context.Context, queue.ProposeUnchangedCodingLedgerCommand) (queue.CodingLedgerRevision, error)
}

func codingLedgerPlanningRoutes(configuration codingagent.Configuration, revision int64) []queue.CodingLedgerRoute {
	values := []struct {
		role  string
		route codingagent.Route
	}{
		{role: "planner", route: configuration.Planner},
		{role: "executor", route: configuration.Executor},
		{role: "reviewer", route: configuration.Reviewer},
	}
	routes := make([]queue.CodingLedgerRoute, len(values))
	for index, value := range values {
		routes[index] = queue.CodingLedgerRoute{
			Role: value.role, ConfigurationRevision: revision,
			Provider: value.route.Provider, Model: value.route.Model,
			ReasoningEffort: value.route.Effort, ServiceTier: value.route.ServiceTier,
		}
	}
	return routes
}

// createCodingLedgerPlanningTransaction persists the first parsed chat draft.
// Later conversation turns update the same PostgreSQL ledger.
func createCodingLedgerPlanningTransaction(
	ctx context.Context,
	repository codingLedgerPlanningRepository,
	controller *codingagent.Controller,
	jobID int64,
	turn codingagent.PlanningTurn,
) (queue.CodingLedgerRevision, error) {
	proposal, err := controller.ProposePlanningLedger(ctx, turn)
	if err != nil {
		return queue.CodingLedgerRevision{}, err
	}
	for index := range proposal.Ledger.Tasks {
		bindPlanningTaskSource(&proposal.Ledger.Tasks[index], nil, turn, 1)
	}
	created, err := repository.CreateCodingLedger(ctx, queue.CreateCodingLedgerCommand{
		JobID: jobID, OperationID: turn.OperationID, OwnerTurnID: turn.OwnerTurnID,
		OwnerTurnText: turn.Text, OwnerTurnDisposition: proposal.OwnerTurnDisposition,
		Ledger: proposal.Ledger,
		Routes: codingLedgerPlanningRoutes(controller.Configuration, 1),
	})
	if err != nil {
		return queue.CodingLedgerRevision{}, err
	}
	return created, nil
}

// applyCodingLedgerPlanningTransaction reloads the authoritative proposal on
// every attempt. It never depends on a prior provider thread or in-memory ledger.
func applyCodingLedgerPlanningTransaction(
	ctx context.Context,
	repository codingLedgerPlanningRepository,
	controller *codingagent.Controller,
	ledgerID int64,
	turn codingagent.PlanningTurn,
	direct *codingagent.DirectPlanningAction,
) (queue.CodingLedgerRevision, error) {
	current, err := readCompleteCodingLedger(ctx, repository, ledgerID)
	if err != nil {
		return queue.CodingLedgerRevision{}, err
	}
	if current.Revision != turn.ObservedRevision {
		return queue.CodingLedgerRevision{}, fmt.Errorf(
			"planning turn observed revision %d but database revision is %d",
			turn.ObservedRevision, current.Revision,
		)
	}
	var mutation codingagent.LedgerMutation
	if direct != nil {
		mutation, err = codingagent.DirectPlanningMutation(turn, *direct)
	} else {
		mutation, err = controller.PlanPlanningMutation(ctx, current.Ledger, turn)
	}
	if err != nil {
		return queue.CodingLedgerRevision{}, err
	}
	if len(mutation.Operations) == 0 {
		if current.State == "revising" {
			return repository.ProposeUnchangedCodingLedger(ctx, queue.ProposeUnchangedCodingLedgerCommand{
				LedgerID: ledgerID, Revision: current.Revision, OperationID: turn.OperationID,
				OwnerTurnID: turn.OwnerTurnID, OwnerTurnText: turn.Text,
			})
		}
		return current, nil
	}
	if direct == nil {
		for index := range mutation.Operations {
			op := &mutation.Operations[index]
			if (op.Kind != model.LedgerMutationAdd && op.Kind != model.LedgerMutationAmend) || op.Item == nil || op.Item.Task == nil {
				continue
			}
			var retained *model.LedgerTask
			for taskIndex := range current.Ledger.Tasks {
				if current.Ledger.Tasks[taskIndex].ID == op.Item.Task.ID {
					retained = &current.Ledger.Tasks[taskIndex]
					break
				}
			}
			bindPlanningTaskSource(op.Item.Task, retained, turn, current.Revision+1)
		}
	}
	applied, err := repository.ApplyCodingLedgerMutation(ctx, queue.ApplyCodingLedgerMutationCommand{
		LedgerID: ledgerID, OwnerTurnText: turn.Text, Mutation: mutation,
	})
	if err != nil {
		return queue.CodingLedgerRevision{}, err
	}
	return applied, nil
}

func readCompleteCodingLedger(
	ctx context.Context,
	repository codingLedgerPlanningRepository,
	ledgerID int64,
) (queue.CodingLedgerRevision, error) {
	const pageSize = 1000
	var complete queue.CodingLedgerRevision
	for offset := 0; ; offset += pageSize {
		page, err := repository.ReadCodingLedger(ctx, ledgerID, queue.CodingLedgerReadOptions{
			Limit: pageSize, Offset: offset,
		})
		if err != nil {
			return queue.CodingLedgerRevision{}, err
		}
		if offset == 0 {
			complete = page
			complete.Ledger.Tasks = nil
		} else if page.Revision != complete.Revision {
			return queue.CodingLedgerRevision{}, fmt.Errorf("coding ledger changed while loading planning context")
		}
		count := codingLedgerPageItemCount(page.Ledger)
		appendCodingLedgerPage(&complete.Ledger, page.Ledger)
		if count < pageSize {
			break
		}
	}
	if err := complete.Ledger.ValidateDraft(); err != nil {
		return queue.CodingLedgerRevision{}, err
	}
	return complete, nil
}

func codingLedgerPageItemCount(ledger model.EngineeringLedger) int { return len(ledger.Tasks) }

func appendCodingLedgerPage(target *model.EngineeringLedger, page model.EngineeringLedger) {
	target.Tasks = append(target.Tasks, page.Tasks...)
}

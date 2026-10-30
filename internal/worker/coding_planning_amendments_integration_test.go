package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

type amendmentTaskProvider struct {
	task            codingagent.Task
	duringExecution func(context.Context) error
	roles           []codingagent.Role
}

func (p *amendmentTaskProvider) Run(ctx context.Context, call codingagent.Assignment, event func(codingagent.NativeEvent) error) (codingagent.Result, error) {
	p.roles = append(p.roles, call.Role)
	if call.Role == codingagent.Execute || call.Role == codingagent.Review {
		if call.Task == nil || !reflect.DeepEqual(*call.Task, p.task) {
			return codingagent.Result{}, fmt.Errorf("pending input changed native frozen packet")
		}
		if call.Role == codingagent.Execute {
			if err := p.duringExecution(ctx); err != nil {
				return codingagent.Result{}, err
			}
		}
	}
	if err := event(codingagent.NativeEvent{Frame: json.RawMessage(`{"fixture":"amendment-boundary"}`)}); err != nil {
		return codingagent.Result{}, err
	}
	content := json.RawMessage(`"Completed the frozen task."`)
	if call.Role == codingagent.ExecutionResponse {
		content = json.RawMessage(`{"kind":"done"}`)
	}
	if call.Role == codingagent.ReviewResponse {
		content = json.RawMessage(`{"kind":"done","findings":[],"verification":"Inspected the exact frozen packet."}`)
	}
	return codingagent.Result{Provider: call.Route.Provider, Model: call.Route.Model, Completed: true, ThreadID: "isolated-" + string(call.Role), TurnID: "turn", Content: content}, nil
}

// Real PostgreSQL and task-loop/replan/planning consumers; scripted provider
// output supplies semantic results. No installed-model judgment is claimed.
func TestPlanningAmendmentWorkerAcceptsCurrentTaskThenPlansOnce(t *testing.T) {
	for _, unchanged := range []bool{false, true} {
		t.Run(fmt.Sprintf("unchanged_%t", unchanged), func(t *testing.T) { testPlanningAmendmentWorker(t, unchanged) })
	}
}
func testPlanningAmendmentWorker(t *testing.T, unchanged bool) {
	url := os.Getenv("OMNI_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required")
	}
	pool, repo := freshWorkerEvidenceRepository(t, url)
	local := newLocalAgentWorkspaceFixture(t, "11111111111111111111111111111111")
	ctx := local.ctx
	route := codingagent.Route{Provider: "native", Model: "fixture-model", Effort: "medium", ServiceTier: "default"}
	configuration := codingagent.Configuration{Mode: codingagent.Agents, Planner: route, LedgerParser: route, Executor: route, Reviewer: route}
	repo.WithCodingAgents(configuration)
	channel, err := repo.EnsureCLIChatSessionChannel(ctx, local.scope.Root, local.scope.Identity)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := model.NewLifecycleOperationID()
	command := queue.ChannelSessionTurnCommand{OperationID: id, ChannelID: channel.ID, WorkspaceRoot: local.scope.Root, WorkspaceIdentity: local.scope.Identity, CodingMode: codingagent.Agents, Text: "Preserve the original approved task."}
	initial, err := repo.SubmitChannelSessionTurn(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	planner, err := repo.ClaimNextStep(ctx, "amendment-initial")
	if err != nil || planner == nil {
		t.Fatal(err)
	}
	ledger := planningBoundaryLedger()
	ledger.Revision = 1
	created, err := repo.CreateCodingLedger(ctx, queue.CreateCodingLedgerCommand{JobID: initial.Job.ID, OperationID: "worker-create", OwnerTurnID: "worker-owner", OwnerTurnText: command.Text, Ledger: ledger, Routes: codingLedgerPlanningRoutes(configuration, 1), OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{"task"})})
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.MarkCodingLedgerReviewReady(ctx, planner.Authority, created.LedgerID, created.Revision); err != nil {
		t.Fatal(err)
	}
	agreed, err := repo.ApplyCodingLedgerControl(ctx, queue.ApplyCodingLedgerMutationCommand{LedgerID: created.LedgerID, OwnerTurnText: "Agree this task.", Mutation: model.LedgerMutation{OperationID: "worker-agree", OwnerTurnID: "worker-agree", BaseRevision: created.Revision, OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{"task"}), Operations: []model.LedgerMutationOperation{{Kind: model.LedgerMutationReview, TargetID: "task", ReviewState: model.LedgerReviewAgreed, Rationale: "Owner agreement."}}}})
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := repo.FreezeCodingLedger(ctx, queue.FreezeCodingLedgerCommand{LedgerID: agreed.LedgerID, Revision: agreed.Revision, OperationID: "worker-start", OwnerTurnID: "worker-start", OwnerTurnText: "Sign off and start."})
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.CompleteCodingLedgerReview(ctx, initial.Job.ID, frozen.LedgerID, frozen.Revision); err != nil {
		t.Fatal(err)
	}
	executor, err := repo.ClaimNextStep(ctx, "amendment-executor")
	if err != nil || executor == nil {
		t.Fatal(err)
	}
	task, err := repo.NextIncompleteCodingLedgerTask(ctx, executor.Authority)
	if err != nil || task == nil {
		t.Fatal(err)
	}
	var messageID model.LifecycleOperationID
	messageConsumed := make(chan struct{}, 1)
	native := &amendmentTaskProvider{task: *task, duringExecution: func(taskCtx context.Context) error {
		message := command
		message.OperationID, _ = model.NewLifecycleOperationID()
		messageID = message.OperationID
		message.ExpectedJobID = &executor.Job.ID
		message.ExpectedGeneration = &executor.Authority.Generation
		message.Text = "Move the task later without changing its accepted work."
		if _, err := repo.SubmitChannelSessionTurn(taskCtx, message); err != nil {
			return err
		}
		select {
		case <-messageConsumed:
		case <-taskCtx.Done():
			return fmt.Errorf("queued message was not consumed during native execution: %w", taskCtx.Err())
		}
		still, err := repo.NextIncompleteCodingLedgerTask(taskCtx, executor.Authority)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(still, task) {
			return fmt.Errorf("pending message changed the frozen task")
		}
		return nil
	}}
	runtime := &nativeRuntimeV3{svc: &Service{repo: repo}, ctx: ctx, claim: executor, action: "v3_coding"}
	runtime.sessionMessages = &channelSessionMessageConsumer{
		claim: executor, repository: repo,
		kind: sessionKindFunc(func(assemblyline.ConversationObjectiveKindInput) (assemblyline.ConversationObjectiveKindDecision, int, error) {
			return assemblyline.ConversationObjectiveKindDecision{Schema: assemblyline.ConversationObjectiveKindSchemaV1, Kind: assemblyline.ObjectiveKindWorkspaceMutation}, 0, nil
		}),
		resolved: func(model.LifecycleOperationID) { messageConsumed <- struct{}{} },
	}
	runCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	controller := &codingagent.Controller{JobID: executor.Job.ID, Generation: executor.Authority.Generation, Root: local.scope.Root, Configuration: configuration, Providers: map[string]codingagent.Provider{"native": native}, Environment: planningBoundaryEnvironment{}, Evidence: repo.CodingAgentEvidence(executor.Authority)}
	if err = runtime.runCodingTaskLoop(runCtx, controller); !errors.Is(err, queue.ErrStaleStepAttempt) {
		t.Fatal("task boundary did not enter replan", err)
	}
	var accepted string
	if err = pool.QueryRow(ctx, `SELECT state FROM coding_agent_tasks WHERE ledger_id=$1 AND frozen_revision=$2 AND task_item_id='task'`, frozen.LedgerID, frozen.Revision).Scan(&accepted); err != nil || accepted != "accepted" {
		t.Fatal("transition preceded task acceptance", err)
	}
	if len(native.roles) != 4 || native.roles[0] != codingagent.Execute || native.roles[2] != codingagent.Review {
		t.Fatal("unexpected native sequence", native.roles)
	}
	next, err := repo.ClaimNextStep(ctx, "amendment-continuation")
	if err != nil || next == nil || next.Step.Action != "v3_coding_plan" || next.Job.ID != initial.Job.ID {
		t.Fatal("same job planning", err)
	}
	proposalProvider := &planningConversationProvider{unchanged: unchanged}
	service, err := New(repo, nil, nil, Options{PollInterval: "1ms", InferenceContextTokens: "8192", HostDirectoryAccessRoot: local.scope.Root, WorkspaceConnections: local.service.workspaceConnections, AgentProviders: func() (map[string]codingagent.Provider, error) {
		return map[string]codingagent.Provider{"native": proposalProvider}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err = service.runClaim(ctx, "amendment-continuation", next); err != nil {
		t.Fatal(err)
	}
	current, _, err := repo.CurrentCodingLedgerForJob(ctx, initial.Job.ID)
	if err != nil || current.LedgerID != frozen.LedgerID || current.Revision != frozen.Revision+1 || current.State != "proposed" || len(current.PendingAmendments) != 0 || len(proposalProvider.calls) != 2 {
		details, _ := repo.CurrentJobDetails(ctx, initial.Job.ID)
		t.Fatalf("queued amendment duplicated or bypassed sign-off: %+v calls%d %v job status=%s error=%s", current, len(proposalProvider.calls), err, details.Job.Status, details.Job.Error)
	}
	snapshot, err := repo.ChannelSessionSnapshot(ctx, channel.ID, queue.MaxChannelSessionMessages, local.scope.Identity)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, turn := range snapshot.Turns {
		if turn.OperationID == messageID {
			found = turn.MessageState == model.ChannelSessionMessagePlanning
		}
	}
	if !found || snapshot.ActiveJob == nil || snapshot.ActiveJob.Job.Status != model.JobStatusWaiting || snapshot.ActiveJob.Job.Instruction != command.Text {
		t.Fatal("same-job sign-off boundary lost")
	}
	if claim, err := repo.ClaimNextStep(ctx, "must-wait-for-owner"); err != nil || claim != nil {
		t.Fatal("execution started without owner sign-off", err)
	}
}

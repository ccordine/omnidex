package worker

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/sessionintent"
)

// Exercise typed intake and the actual worker continuation, with only the
// provider's semantic output scripted. All state transitions use PostgreSQL.
func TestInteractWorkerContinuationExtractsLedger(t *testing.T) {
	url := os.Getenv("OMNI_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required")
	}
	_, repository := freshWorkerEvidenceRepository(t, url)
	fixture := newLocalAgentWorkspaceFixture(t, "11111111111111111111111111111111")
	ctx := fixture.ctx
	route := codingagent.Route{Provider: "native", Model: "fixture-model", Effort: "medium", ServiceTier: "default"}
	configuration := codingagent.Configuration{Mode: codingagent.Agents, Planner: route, LedgerParser: route, Executor: route, Reviewer: route}
	repository.WithCodingAgents(configuration)
	channel, err := repository.EnsureCLIChatSessionChannel(ctx, fixture.scope.Root, fixture.scope.Identity)
	if err != nil {
		t.Fatal(err)
	}
	operation, err := model.NewLifecycleOperationID()
	if err != nil {
		t.Fatal(err)
	}
	initial, err := repository.SubmitChannelSessionTurn(ctx, queue.ChannelSessionTurnCommand{OperationID: operation, ChannelID: channel.ID, WorkspaceRoot: fixture.scope.Root, WorkspaceIdentity: fixture.scope.Identity, CodingMode: codingagent.Agents, Text: "Implement the requested task."})
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "interact-initial")
	if err != nil || claim == nil {
		t.Fatal(claim, err)
	}
	ledger := planningBoundaryLedger()
	ledger.Revision = 1
	created, err := repository.CreateCodingLedger(ctx, queue.CreateCodingLedgerCommand{JobID: initial.Job.ID, OperationID: string(operation) + "_create", OwnerTurnID: "initial", OwnerTurnText: initial.Job.Instruction, Ledger: ledger, Routes: codingLedgerPlanningRoutes(configuration, 1), OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{"task"})})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.MarkCodingLedgerReviewReady(ctx, claim.Authority, created.LedgerID, created.Revision); err != nil {
		t.Fatal(err)
	}
	state, err := repository.ChannelSessionState(ctx, channel.ID, fixture.scope.Identity)
	if err != nil {
		t.Fatal(err)
	}
	operation, err = model.NewLifecycleOperationID()
	if err != nil {
		t.Fatal(err)
	}
	text := "  Move the task later.\nKeep the scope unchanged.  "
	request := sessionintent.Request{OperationID: operation, Mode: sessionintent.Interact, Recipient: sessionintent.Planner, Text: text, State: state, LedgerRevision: created.Revision}
	_, err = repository.InterpretUserIntent(ctx, request, func(_ context.Context, current sessionintent.Context) (sessionintent.Result, json.RawMessage, error) {
		result, err := sessionintent.DirectContent(request, current)
		return result, json.RawMessage(`[]`), err
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := repository.ExecuteUserIntent(ctx, operation, channel.ID, fixture.scope.Identity)
	if err != nil || receipt.JobID != initial.Job.ID || receipt.Generation != initial.Job.CurrentGeneration+1 {
		t.Fatal(receipt, err)
	}
	continuation, err := repository.ClaimNextStep(ctx, "interact-continuation")
	if err != nil || continuation == nil || continuation.Step.Action != "v3_coding_plan" {
		t.Fatal(continuation, err)
	}
	provider := &planningConversationProvider{}
	service, err := New(repository, nil, nil, Options{PollInterval: "1ms", InferenceContextTokens: "8192", HostDirectoryAccessRoot: fixture.scope.Root, WorkspaceConnections: fixture.service.workspaceConnections, AgentProviders: func() (map[string]codingagent.Provider, error) {
		return map[string]codingagent.Provider{"native": provider}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.runClaim(ctx, "interact-continuation", continuation); err != nil {
		t.Fatal(err)
	}
	if len(provider.calls) != 2 || provider.calls[0].Role != codingagent.Plan || provider.calls[1].Role != codingagent.PlanningMutation {
		t.Fatalf("expected planner then ledger extraction: %+v", provider.calls)
	}
	if provider.calls[0].Objective.Instruction != "user redirect:\n"+text || provider.calls[1].PlanningResponse != "I updated the task order as requested." || provider.calls[1].PlanningBaseRevision != created.Revision {
		t.Fatalf("extraction lost exact continuation context: %+v", provider.calls)
	}
	current, exists, err := repository.CurrentCodingLedgerForJob(ctx, initial.Job.ID)
	if err != nil || !exists || current.LedgerID != created.LedgerID || current.Revision != created.Revision+1 || current.State != "proposed" || current.Ledger.Tasks[0].Rank != 10 || current.Ledger.Tasks[0].Goal != ledger.Tasks[0].Goal || current.Ledger.Tasks[0].ReviewState != model.LedgerReviewProposed {
		t.Fatalf("extraction did not persist an unapproved same-ledger mutation: %+v %v", current, err)
	}
	details, err := repository.CurrentJobDetails(ctx, initial.Job.ID)
	if err != nil || details.Job.Status != model.JobStatusWaiting {
		t.Fatal(details, err)
	}
	if next, err := repository.ClaimNextStep(ctx, "interact-no-execution"); err != nil || next != nil {
		t.Fatalf("unapproved content dispatched execution: %+v %v", next, err)
	}
}

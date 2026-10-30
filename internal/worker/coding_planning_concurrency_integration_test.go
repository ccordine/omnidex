package worker

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

type racingPlanningProvider struct {
	planningConversationProvider
	racing func()
	amend  *model.LedgerTask
}

func (p *racingPlanningProvider) Run(ctx context.Context, call codingagent.Assignment, event func(codingagent.NativeEvent) error) (codingagent.Result, error) {
	result, err := p.planningConversationProvider.Run(ctx, call, event)
	if call.Role == codingagent.PlanningMutation && p.amend != nil {
		result.Content, err = json.Marshal(map[string]any{"operations": []model.LedgerMutationOperation{{Kind: model.LedgerMutationAmend, TargetID: p.amend.ID, Item: &model.LedgerMutationItem{Task: p.amend}}}})
	}
	if call.Role == codingagent.PlanningMutation && p.racing != nil {
		p.racing()
		p.racing = nil
	}
	return result, err
}

func TestPlanningWorkerRetainsConcurrentOwnerControls(t *testing.T) {
	for _, amend := range []bool{false, true} {
		name := "order"
		if amend {
			name = "task-content"
		}
		t.Run(name, func(t *testing.T) { testPlanningWorkerRetainsConcurrentOwnerControls(t, amend) })
	}
}

func testPlanningWorkerRetainsConcurrentOwnerControls(t *testing.T, amend bool) {
	url := os.Getenv("OMNI_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required")
	}
	_, repo := freshWorkerEvidenceRepository(t, url)
	local := newLocalAgentWorkspaceFixture(t, "11111111111111111111111111111111")
	ctx := local.ctx
	route := codingagent.Route{Provider: "native", Model: "fixture-model", Effort: "medium", ServiceTier: "default"}
	config := codingagent.Configuration{Mode: codingagent.Agents, Planner: route, LedgerParser: route, Executor: route, Reviewer: route}
	repo.WithCodingAgents(config)
	channel, err := repo.EnsureCLIChatSessionChannel(ctx, local.scope.Root, local.scope.Identity)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := model.NewLifecycleOperationID()
	initial, err := repo.SubmitChannelSessionTurn(ctx, queue.ChannelSessionTurnCommand{OperationID: id, ChannelID: channel.ID, WorkspaceRoot: local.scope.Root, WorkspaceIdentity: local.scope.Identity, CodingMode: codingagent.Agents, Text: "Keep this task."})
	if err != nil {
		t.Fatal(err)
	}
	planner, err := repo.ClaimNextStep(ctx, "concurrent-planner")
	if err != nil || planner == nil {
		t.Fatalf("claim: %v %v", planner, err)
	}
	ledger := planningBoundaryLedger()
	ledger.Revision = 1
	created, err := repo.CreateCodingLedger(ctx, queue.CreateCodingLedgerCommand{JobID: initial.Job.ID, OperationID: "create", OwnerTurnID: "create", OwnerTurnText: "Keep this task.", Ledger: ledger, Routes: codingLedgerPlanningRoutes(config, 1), OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{"task"})})
	if err != nil {
		t.Fatal(err)
	}
	control := func(revision, rank int64, op string) queue.CodingLedgerRevision {
		t.Helper()
		result, err := repo.ApplyCodingLedgerControl(ctx, queue.ApplyCodingLedgerMutationCommand{LedgerID: created.LedgerID, OwnerTurnText: "Set the displayed task priority.", Mutation: model.LedgerMutation{OperationID: op, OwnerTurnID: op, BaseRevision: revision, OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{"task"}), Operations: []model.LedgerMutationOperation{{Kind: model.LedgerMutationReorder, TargetID: "task", Rank: &rank}}}})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	p := &racingPlanningProvider{racing: func() { control(created.Revision, 42, "newer-order") }}
	if amend {
		task := ledger.Tasks[0]
		task.Objective = "Preserve the requested additional task detail."
		p.amend = &task
	}
	c := &codingagent.Controller{JobID: initial.Job.ID, Generation: planner.Authority.Generation, Root: local.scope.Root, Configuration: config, Providers: map[string]codingagent.Provider{"native": p}, Environment: planningBoundaryEnvironment{}, Evidence: repo.CodingAgentEvidence(planner.Authority)}
	turn := codingagent.PlanningTurn{OperationID: "older-turn", OwnerTurnID: "older-turn", Text: "Move the task to position ten.", ObservedRevision: created.Revision}
	result, err := applyCodingLedgerPlanningTransaction(ctx, repo, c, created.LedgerID, turn, nil)
	if err != nil || result.Ledger.Tasks[0].Rank != 42 || len(p.calls) != 2 {
		t.Fatalf("lost newer order or repeated models: result=%+v calls=%d err=%v", result, len(p.calls), err)
	}
	if amend && (result.Ledger.Tasks[0].Objective != p.amend.Objective || result.Ledger.Tasks[0].ReviewState != model.LedgerReviewProposed || len(result.Ledger.Tasks[0].OwnerContext) != len(p.amend.OwnerContext)) {
		t.Fatal("concurrent control discarded task detail or approved changed content")
	}
	// Publication and the waiting transition may also race with later controls.
	control(result.Revision, 43, "before-publication")
	result, err = publishCurrentPlanningLedger(ctx, repo, c, turn, result)
	if err != nil || result.Ledger.Tasks[0].Rank != 43 {
		t.Fatalf("stale publication: %+v %v", result, err)
	}
	control(result.Revision, 44, "before-waiting")
	result, err = finishPlanningConversation(ctx, repo, planner.Authority, c, result)
	if err != nil || result.Ledger.Tasks[0].Rank != 44 {
		t.Fatalf("stale waiting transition: %+v %v", result, err)
	}
	details, err := repo.CurrentJobDetails(ctx, initial.Job.ID)
	if err != nil || details.Job.Status != model.JobStatusWaiting {
		t.Fatalf("concurrent control failed job: %+v %v", details.Job, err)
	}
	if len(p.calls) != 2 {
		t.Fatalf("mechanical controls caused extra model calls: %d", len(p.calls))
	}
}

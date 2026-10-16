package queue

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestCapturedTaskLedgerFreezesWithoutAnInventedGlobalObjective(t *testing.T) {
	verifyCapturedTaskLedgerFreeze(t, "wordcounter-freeze.json", 3)
}

func TestCapturedReadAndEditTaskLedgerFreezes(t *testing.T) {
	verifyCapturedTaskLedgerFreeze(t, "wordcounter-readwrite.json", 1)
}

func verifyCapturedTaskLedgerFreeze(t *testing.T, fixture string, expectedCriteria int) {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/planning-gauntlet/" + fixture)
	if err != nil {
		t.Fatal(err)
	}
	var capture struct {
		Instruction      string `json:"instruction"`
		PlanningResponse string `json:"planning_response"`
		Response         struct {
			Ledger model.EngineeringLedger `json:"ledger"`
		} `json:"parser_response"`
	}
	if err := json.Unmarshal(raw, &capture); err != nil {
		t.Fatal(err)
	}
	ledger := capture.Response.Ledger
	ledger.ID, ledger.Revision = "captured-wordcounter", 1
	if len(ledger.Objectives) != 0 || len(ledger.Tasks) != 3 {
		t.Fatal("captured failed plan was changed")
	}
	_, repository := freshCodingPlanRepository(t)
	startup := codingAgentTestConfiguration()
	startup.LedgerParser = codingagent.Route{Provider: "codex", Model: "recorded-parser", Effort: "low", ServiceTier: "default"}
	repository.WithCodingAgents(startup)
	ctx := context.Background()
	root := t.TempDir()
	job, err := repository.EnqueueCodingJob(ctx, capture.Instruction, root)
	if err != nil {
		t.Fatal(err)
	}
	planner, err := repository.ClaimNextStep(ctx, "captured-freeze")
	if err != nil || planner == nil {
		t.Fatalf("planner claim: %v", err)
	}
	configuration, err := codingagent.ConfigurationFromMetadata(planner.Job.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	controller := codingagent.Controller{JobID: job.ID, Generation: planner.Authority.Generation, Root: root,
		Requirements:  []codingagent.Requirement{{ID: "objective", Outcome: capture.Instruction}},
		Configuration: configuration, Providers: map[string]codingagent.Provider{"codex": capturedFreezeParser{capture.PlanningResponse, ledger}},
		Evidence: repository.CodingAgentEvidence(planner.Authority), Environment: capturedFreezeEnvironment{}}
	proposal, err := controller.ProposePlanningLedger(ctx, codingagent.PlanningTurn{OperationID: "captured-proposal", OwnerTurnID: "captured-plan-owner", Text: capture.Instruction})
	if err != nil {
		t.Fatal(err)
	}
	ledger = proposal.Ledger
	for _, criterion := range ledger.AcceptanceCriteria {
		if !criterion.Required {
			t.Fatal("parser chose optional acceptance")
		}
	}
	for _, check := range ledger.Checks {
		if !check.Required {
			t.Fatal("parser chose optional verification")
		}
	}
	ids := []model.LedgerItemID{}
	for _, record := range ledgerRepositoryRecords(ledger) {
		ids = append(ids, record.item.ID)
	}
	for _, edge := range ledger.Dependencies {
		ids = append(ids, edge.ID)
	}
	created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{JobID: job.ID,
		OperationID: "captured-create", OwnerTurnID: "captured-owner", OwnerTurnText: capture.Instruction,
		Ledger: ledger, Routes: repositoryLedgerRoutes(1), OwnerTurnDisposition: model.RepresentedOwnerTurn(ids)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.FreezeCodingLedger(ctx, FreezeCodingLedgerCommand{LedgerID: created.LedgerID, Revision: created.Revision,
		OperationID: "unapproved-freeze", OwnerTurnID: "owner-unapproved", OwnerTurnText: "Freeze before agreement."}); err == nil {
		t.Fatal("removing the obsolete objective gate bypassed owner agreement")
	}
	frozen := freezeCodingAgentPlanFixture(t, repository, job, codingAgentLedgerFixture{
		authority: planner.Authority, created: created, itemIDs: ids, goal: capture.Instruction})
	if frozen.State != "frozen" || len(frozen.Ledger.Objectives) != 0 || len(frozen.Ledger.Tasks) != 3 {
		t.Fatal("freeze invented scope or lost tasks")
	}
	execution, err := repository.ClaimNextStep(ctx, "captured-executor")
	if err != nil || execution == nil {
		t.Fatalf("execution claim: %v", err)
	}
	changedDefaults := startup
	changedDefaults.LedgerParser.Model = "later-default-parser"
	repository.WithCodingAgents(changedDefaults)
	frozenConfiguration, err := repository.FrozenCodingLedgerConfiguration(ctx, execution.Authority)
	if err != nil || frozenConfiguration.LedgerParser != configuration.LedgerParser {
		t.Fatalf("frozen acceptance lost the exact immutable parser route: %+v %v", frozenConfiguration, err)
	}
	task, err := repository.NextIncompleteCodingLedgerTask(ctx, execution.Authority)
	if err != nil || task == nil || task.ID != "t1" || len(task.Acceptance) != expectedCriteria {
		t.Fatalf("first exact task packet lost its criteria: %+v %v", task, err)
	}
	if fixture == "wordcounter-readwrite.json" && (len(task.ReadFiles) != 1 || len(task.WriteFiles) != 1 || task.ReadFiles[0] != task.WriteFiles[0]) {
		t.Fatal("captured read/edit context was stripped instead of preserved")
	}
}

// Recorded prose/parser bytes qualify deterministic mapping and PG freeze,
// not native inference or application behavior.
type capturedFreezeParser struct {
	prose  string
	ledger model.EngineeringLedger
}

func (provider capturedFreezeParser) Run(_ context.Context, call codingagent.Assignment, event func(codingagent.NativeEvent) error) (codingagent.Result, error) {
	var content []byte
	if call.Role == codingagent.Plan {
		content, _ = json.Marshal(provider.prose)
	} else {
		content, _ = json.Marshal(map[string]any{"ledger": provider.ledger})
	}
	if err := event(codingagent.NativeEvent{Frame: json.RawMessage(`{"captured_replay":true}`)}); err != nil {
		return codingagent.Result{}, err
	}
	return codingagent.Result{Provider: call.Route.Provider, Model: call.Route.Model, ThreadID: "captured-thread", TurnID: "captured-turn", Completed: true, Content: content}, nil
}

type capturedFreezeEnvironment struct{}

func (capturedFreezeEnvironment) Attest(context.Context) error { return nil }

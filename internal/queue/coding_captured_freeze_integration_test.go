package queue

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/llm"
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
			Ledger struct {
				model.EngineeringLedger
				Tasks []struct {
					model.LedgerTask
					AcceptanceIDs  []model.LedgerItemID `json:"acceptance_criterion_ids"`
					RequirementIDs []model.LedgerItemID `json:"requirement_ids"`
					ConstraintIDs  []model.LedgerItemID `json:"constraint_ids"`
					Delivery       string               `json:"delivery_obligation"`
				} `json:"tasks"`
				Requirements []struct {
					ID         model.LedgerItemID     `json:"id"`
					Outcome    string                 `json:"outcome"`
					Provenance model.LedgerProvenance `json:"provenance"`
				} `json:"requirements"`
				Criteria []struct {
					ID      model.LedgerItemID `json:"id"`
					Outcome string             `json:"outcome"`
				} `json:"acceptance_criteria"`
				Checks []struct {
					Criteria    []model.LedgerItemID `json:"acceptance_criterion_ids"`
					Observation string               `json:"observation"`
					Argv        []string             `json:"argv"`
				} `json:"checks"`
				Constraints []struct {
					ID   model.LedgerItemID `json:"id"`
					Rule string             `json:"rule"`
				} `json:"constraints"`
			} `json:"ledger"`
		} `json:"parser_response"`
	}
	if err := json.Unmarshal(raw, &capture); err != nil {
		t.Fatal(err)
	}
	// Historical capture normalization is fixture data preparation only.
	ledger := model.EngineeringLedger{}
	for _, old := range capture.Response.Ledger.Tasks {
		task := old.LedgerTask
		if old.Delivery != "" {
			task.Goal += "\n\n" + old.Delivery
		}
		for _, id := range old.RequirementIDs {
			for _, requirement := range capture.Response.Ledger.Requirements {
				if id == requirement.ID {
					task.OwnerContext = append(task.OwnerContext, model.TaskContext{Text: requirement.Outcome, Provenance: requirement.Provenance})
				}
			}
		}
		for _, id := range old.ConstraintIDs {
			for _, constraint := range capture.Response.Ledger.Constraints {
				if id == constraint.ID {
					task.Constraints = append(task.Constraints, constraint.Rule)
				}
			}
		}
		for _, id := range old.AcceptanceIDs {
			criterion := model.TaskAcceptance{ID: string(id)}
			for _, oldCriterion := range capture.Response.Ledger.Criteria {
				if id == oldCriterion.ID {
					criterion.Text = oldCriterion.Outcome
				}
			}
			for _, check := range capture.Response.Ledger.Checks {
				for _, linked := range check.Criteria {
					if linked == id {
						verification := check.Observation
						if len(check.Argv) > 0 {
							verification = strings.Join(check.Argv, " ")
						}
						if verification != "" {
							criterion.Verification = append(criterion.Verification, verification)
						}
					}
				}
			}
			task.Acceptance = append(task.Acceptance, criterion)
		}
		ledger.Tasks = append(ledger.Tasks, task)
	}
	ledger.ID, ledger.Revision = "captured-wordcounter", 1
	if len(ledger.Tasks) != 3 {
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
		Configuration: configuration, Providers: map[string]codingagent.Provider{"codex": capturedFreezeParser{capture.PlanningResponse, ledger}},
		Evidence: repository.CodingAgentEvidence(planner.Authority), Environment: capturedFreezeEnvironment{}}
	proposal, err := controller.ProposePlanningLedger(ctx, codingagent.PlanningTurn{OperationID: "captured-proposal", OwnerTurnID: "captured-plan-owner", Text: capture.Instruction})
	if err != nil {
		t.Fatal(err)
	}
	ledger = proposal.Ledger
	for _, task := range ledger.Tasks {
		if len(task.Acceptance) == 0 {
			t.Fatal("captured acceptance lost")
		}
		for _, criterion := range task.Acceptance {
			if len(criterion.Verification) == 0 {
				t.Fatal("captured verification lost")
			}
		}
	}
	ids := []model.LedgerItemID{}
	for _, record := range ledgerRepositoryRecords(ledger) {
		ids = append(ids, record.item.ID)
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
	if frozen.State != "frozen" || len(frozen.Ledger.Tasks) != 3 {
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
	if !reflect.DeepEqual(task.Acceptance, ledger.Tasks[0].Acceptance) || !reflect.DeepEqual(task.Constraints, ledger.Tasks[0].Constraints) || !reflect.DeepEqual(task.OwnerContext, ledger.Tasks[0].OwnerContext) {
		t.Fatal("captured task-owned obligations changed during freeze")
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

func (provider capturedFreezeParser) OrderingClient(context.Context) (llm.ExactStationClient, func(), error) {
	return provider, func() {}, nil
}

func (provider capturedFreezeParser) GeneratePreparedExact(_ context.Context, prepared llm.PreparedModel) (llm.PreparedGeneration, error) {
	// The historical capture's explicit task order is scripted semantic output.
	ids := []model.LedgerItemID{}
	for _, task := range provider.ledger.Tasks {
		ids = append(ids, task.ID)
	}
	sequence, err := json.Marshal(ids)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	raw, err := json.Marshal(map[string]any{"created_at": "2026-10-20T12:00:00Z", "response": string(sequence), "done": true, "done_reason": "stop", "prompt_eval_count": 40, "eval_count": 12})
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	decoded, err := llm.DecodeExactPreparedResponseForProtocol(prepared.Protocol, 200, raw)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	return llm.PreparedGeneration{Schema: llm.PreparedGenerationSchemaV1, Protocol: prepared.Protocol, ProviderRequestDisposition: llm.ProviderRequestDispatched, Content: decoded.Content, ProviderHTTPStatus: 200, ProviderResponseDisposition: decoded.Disposition, ProviderResponseComplete: true, ProviderContentEncoding: llm.ProviderEncodingIdentity, ProviderResponseBytesKnown: true, ProviderResponseBytes: int64(len(raw)), ProviderResponseCapturedBytes: len(raw), ProviderResponseCapture: raw, ProviderDonePresent: decoded.DonePresent, ProviderDone: decoded.Done, ProviderDoneReason: decoded.DoneReason, UsagePresent: decoded.UsagePresent, Usage: decoded.Usage}, nil
}

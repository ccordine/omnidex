package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

type sieveGoldenCase struct {
	Name, Role, Source, Owner string
	Criteria                  []string
	Expected                  struct {
		Tasks                            *int
		Kind                             string
		Findings                         *int
		Required, Constraints, Forbidden []string
	}
}

// Held-out positive and forbidden-result assertions qualify one selected route.
// Repeating the same corpus across routes benchmarks the production parser.
func TestInstalledParserSieveGoldenCorpus(t *testing.T) {
	if os.Getenv("OMNI_TEST_PARSER_SIEVE") != "1" {
		t.Skip("OMNI_TEST_PARSER_SIEVE=1 is required")
	}
	client, route := installedSieveProvider(t)
	qualification, stop := context.WithTimeout(t.Context(), 20*time.Minute)
	defer stop()
	modelCalls := 0
	raw, err := os.ReadFile("../../testdata/parser-sieve/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct{ Cases []sieveGoldenCase }
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range corpus.Cases {
		t.Run(fixture.Name, func(t *testing.T) {
			call := codingagent.Assignment{JobID: 17, Generation: 1, Role: codingagent.Role(fixture.Role), Root: t.TempDir(),
				Route: route}
			var before codingagent.EngineeringLedger
			if call.Role.IsPlanning() {
				call.PlanningResponse = fixture.Source
				context := map[string]any{"owner_turn": codingagent.PlanningTurn{OperationID: "fixture", OwnerTurnID: "owner", Text: fixture.Owner}}
				if call.Role == codingagent.PlanningMutation {
					before = sieveGoldenLedger()
					call.PlanningBaseRevision = before.Revision
					context["ledger"] = before
				}
				encoded, _ := json.Marshal(context)
				call.ConstructionFacts = []codingagent.ConstructionFact{{Kind: "bounded_planning_context", Quote: string(encoded)}}
			} else {
				call.Response = fixture.Source
				if call.Role == codingagent.ReviewResponse {
					call.Task = &codingagent.Task{ID: "fixture-task", Acceptance: fixture.Criteria}
				}
			}
			frames := []json.RawMessage{}
			frameSeconds := []float64{}
			envelopes := []sieveBenchmarkEnvelope{}
			ctx, cancel := context.WithTimeout(qualification, 5*time.Minute)
			defer cancel()
			start := time.Now()
			result, runErr := client.Run(ctx, call, func(event codingagent.NativeEvent) error {
				if len(event.Frame) == 0 {
					return nil
				}
				frames = append(frames, append(json.RawMessage(nil), event.Frame...))
				frameSeconds = append(frameSeconds, time.Since(start).Seconds())
				envelope, err := sieveBenchmarkEnvelopeFor(call, event.Frame)
				if err != nil {
					return err
				}
				if envelope != nil {
					envelopes = append(envelopes, *envelope)
				}
				var receipt struct {
					Result struct {
						InstructionSources []string `json:"instructionSources"`
					} `json:"result"`
					Provider string `json:"provider"`
					Phase    string `json:"phase"`
				}
				if err := json.Unmarshal(event.Frame, &receipt); err != nil {
					return err
				}
				if len(receipt.Result.InstructionSources) > 0 {
					return fmt.Errorf("parser inherited external instruction files")
				}
				if receipt.Result.InstructionSources != nil || receipt.Provider == "ollama" && receipt.Phase == "request" {
					modelCalls++
					if modelCalls > 160 {
						return fmt.Errorf("qualification exceeded its 160-call budget")
					}
				}
				return nil
			})
			if evidence := os.Getenv("OMNI_PARSER_SIEVE_EVIDENCE"); evidence != "" {
				encoded, _ := json.Marshal(map[string]any{"assignment": call, "result": result, "frames": frames, "frame_seconds": frameSeconds, "envelopes": envelopes, "seconds": time.Since(start).Seconds(), "error": fmt.Sprint(runErr)})
				if err := os.WriteFile(filepath.Join(evidence, fixture.Name+".json"), encoded, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if runErr != nil {
				t.Fatal(runErr)
			}
			if err := result.ValidateFor(call.Role, call.Route); err != nil {
				t.Fatal(err)
			}
			assertSieveGolden(t, fixture, result, before)
			t.Logf("qualified %s with %d original/code receipts", fixture.Name, len(frames))
		})
	}
}

func assertSieveGolden(t *testing.T, fixture sieveGoldenCase, result codingagent.Result, before codingagent.EngineeringLedger) {
	t.Helper()
	if fixture.Role == "execution_response" {
		var outcome struct{ Kind string }
		if err := json.Unmarshal(result.Content, &outcome); err != nil || outcome.Kind != fixture.Expected.Kind {
			t.Fatalf("expected %s: %s %v", fixture.Expected.Kind, result.Content, err)
		}
		return
	}
	if fixture.Role == "review_response" {
		var review codingagent.ReviewResponseResult
		if err := json.Unmarshal(result.Content, &review); err != nil || string(review.Kind) != fixture.Expected.Kind || len(review.Findings) != *fixture.Expected.Findings {
			t.Fatalf("incorrect review result: %s %v", result.Content, err)
		}
		for _, criterion := range fixture.Criteria {
			if *fixture.Expected.Findings == len(fixture.Criteria) {
				found := false
				for _, f := range review.Findings {
					found = found || f.Requirement == criterion && f.TaskID == "fixture-task"
				}
				if !found {
					t.Fatalf("omitted or misbound defect for %q", criterion)
				}
			}
		}
		return
	}
	var ledger codingagent.EngineeringLedger
	if fixture.Role == "planning_initial" {
		var parsed struct{ Ledger codingagent.EngineeringLedger }
		if err := json.Unmarshal(result.Content, &parsed); err != nil {
			t.Fatal(err)
		}
		ledger = parsed.Ledger
	} else {
		var mutation codingagent.LedgerMutation
		if err := json.Unmarshal(result.Content, &mutation); err != nil {
			t.Fatal(err)
		}
		mutation.OperationID, mutation.OwnerTurnID, mutation.BaseRevision = "fixture", "owner", before.Revision
		ids := []model.LedgerItemID{}
		for _, op := range mutation.Operations {
			if op.TargetID != "" {
				ids = append(ids, op.TargetID)
			} else if op.Item != nil && op.Item.Base() != nil {
				ids = append(ids, op.Item.Base().ID)
			}
		}
		mutation.OwnerTurnDisposition = model.RepresentedOwnerTurn(ids)
		var err error
		ledger, err = codingagent.ApplyPlanningMutation(before, mutation)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := ledger.Validate(); err != nil {
		t.Fatalf("missing complete requirement/criterion/check coverage: %v", err)
	}
	if len(ledger.Tasks) != *fixture.Expected.Tasks {
		t.Fatalf("expected %d tasks, got %d: %s", *fixture.Expected.Tasks, len(ledger.Tasks), result.Content)
	}
	encoded, _ := json.Marshal(ledger)
	for _, text := range fixture.Expected.Required {
		if !strings.Contains(string(encoded), text) {
			t.Fatalf("lost required information %q: %s", text, result.Content)
		}
	}
	if fixture.Role == "planning_initial" && len(fixture.Expected.Required) == len(ledger.Tasks) {
		matched := map[model.LedgerItemID]bool{}
		for _, text := range fixture.Expected.Required {
			found := false
			for _, task := range ledger.Tasks {
				if strings.Contains(task.Goal, text) && !matched[task.ID] {
					matched[task.ID], found = true, true
					break
				}
			}
			if !found {
				t.Fatalf("obligation %q did not have its own task", text)
			}
		}
	}
	for _, text := range fixture.Expected.Forbidden {
		if strings.Contains(string(encoded), text) {
			t.Fatalf("forbidden material entered ledger: %q", text)
		}
	}
	for _, text := range fixture.Expected.Constraints {
		found := false
		for _, rule := range ledger.Constraints {
			if strings.Contains(rule.Rule, text) {
				found = true
				for _, task := range ledger.Tasks {
					bound := false
					for _, id := range task.ConstraintIDs {
						bound = bound || id == rule.ID
					}
					if !bound {
						t.Fatalf("constraint omitted for task %s", task.ID)
					}
				}
			}
		}
		if !found {
			t.Fatalf("lost required policy %q", text)
		}
	}
	if fixture.Name == "mutation-context-not-replacement" {
		for index, old := range before.Tasks {
			after := ledger.Tasks[index]
			if old.ID == "task-a" {
				if !strings.Contains(after.DeliveryObligation, old.DeliveryObligation) {
					t.Fatal("amendment lost prior delivery obligation")
				}
				after.DeliveryObligation = old.DeliveryObligation
				after.ReviewState = old.ReviewState
			}
			if !reflect.DeepEqual(old, after) {
				t.Fatalf("amendment rewrote unrelated task fields: before=%+v after=%+v", old, after)
			}
		}
	}
}

func sieveGoldenLedger() codingagent.EngineeringLedger {
	ledger := codingagent.EngineeringLedger{ID: "fixture-ledger", Revision: 1}
	for index, goal := range []string{"Create the local archive.", "Verify restoration recovers the original bytes."} {
		id := fmt.Sprintf("task-%c", 'a'+index)
		r, ac, ch := id+"-requirement", id+"-criterion", id+"-check"
		base := func(value string) model.LedgerItem {
			return model.LedgerItem{ID: model.LedgerItemID(value), Provenance: model.LedgerProvenance{Kind: "fixture", Reference: "saved-plan", Revision: 1}, Priority: model.LedgerPriorityNormal, Rank: int64(index + 1), Disposition: model.LedgerDispositionActive, ReviewState: model.LedgerReviewAgreed}
		}
		ledger.Requirements = append(ledger.Requirements, model.LedgerRequirement{LedgerItem: base(r), Outcome: goal})
		ledger.Tasks = append(ledger.Tasks, model.LedgerTask{LedgerItem: base(id), Goal: goal, RequirementIDs: []model.LedgerItemID{model.LedgerItemID(r)}, AcceptanceCriterionIDs: []model.LedgerItemID{model.LedgerItemID(ac)}, DeliveryObligation: "Keep existing files and local delivery."})
		ledger.AcceptanceCriteria = append(ledger.AcceptanceCriteria, model.LedgerAcceptanceCriterion{LedgerItem: base(ac), Outcome: goal, RequirementIDs: []model.LedgerItemID{model.LedgerItemID(r)}, TaskIDs: []model.LedgerItemID{model.LedgerItemID(id)}, Required: true})
		ledger.Checks = append(ledger.Checks, model.LedgerCheck{LedgerItem: base(ch), Observation: goal, RequirementIDs: []model.LedgerItemID{model.LedgerItemID(r)}, AcceptanceCriterionIDs: []model.LedgerItemID{model.LedgerItemID(ac)}, Required: true})
	}
	return ledger
}

package codingagent

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestPlanningSieveControllerHandoffPreservesRetainedRows(t *testing.T) {
	for _, name := range []string{"expense", "wordcounter"} {
		t.Run(name, func(t *testing.T) {
			ledger := handoffFixtureLedger(t, name)
			before, _ := json.Marshal(ledger)
			task := ledger.Tasks[0]
			response := "Keep the existing task and include the clarified delivery note.\n\nGive that same task high priority."
			controller := handoffController(t, response, func(call Assignment) any {
				step := call.ParserStep
				if step.Stage == "scan" {
					return scanOracle(step.Source, "change")
				}
				patch := map[string]any{"delivery_obligation": "clarified delivery note"}
				if strings.Contains(step.Source, "high priority") {
					patch = map[string]any{"priority": "high"}
				}
				return handoffAmendment(task.ID, patch)
			})
			turn := PlanningTurn{OperationID: "handoff", OwnerTurnID: "owner", Text: response, ObservedRevision: ledger.Revision}
			mutation, err := controller.PlanPlanningMutation(t.Context(), ledger, turn)
			if err != nil {
				t.Fatal(err)
			}
			if len(mutation.Operations) != 2 {
				t.Fatalf("lost a source statement: %+v", mutation.Operations)
			}
			next, err := ApplyPlanningMutation(ledger, mutation)
			if err != nil {
				t.Fatal(err)
			}
			changed := next.Tasks[0]
			if changed.DeliveryObligation != "clarified delivery note" || changed.Priority != model.LedgerPriorityHigh {
				t.Fatalf("sequential amendments lost their effects: %+v", changed)
			}
			changed.DeliveryObligation, changed.Priority, changed.ReviewState = task.DeliveryObligation, task.Priority, task.ReviewState
			if !reflect.DeepEqual(changed, task) {
				t.Fatalf("handoff reconstructed retained identity, provenance, rank or relationships: %+v", changed)
			}
			after, _ := json.Marshal(ledger)
			if string(before) != string(after) {
				t.Fatal("handoff mutated the authoritative input ledger")
			}
		})
	}
}

func TestPlanningSieveControllerHandoffRejectsModelOwnedMetadata(t *testing.T) {
	ledger := handoffFixtureLedger(t, "expense")
	before, _ := json.Marshal(ledger)
	for _, field := range []string{"id", "rank", "provenance", "disposition", "review_state"} {
		t.Run(field, func(t *testing.T) {
			response := "Clarify the delivery note for the existing task."
			controller := handoffController(t, response, func(call Assignment) any {
				if call.ParserStep.Stage == "scan" {
					return scanOracle(call.ParserStep.Source, "change")
				}
				return handoffAmendment(ledger.Tasks[0].ID, map[string]any{"delivery_obligation": "clarified", field: nil})
			})
			turn := PlanningTurn{OperationID: "forbidden-" + field, OwnerTurnID: "owner", Text: response, ObservedRevision: ledger.Revision}
			if _, err := controller.PlanPlanningMutation(t.Context(), ledger, turn); err == nil || !strings.Contains(err.Error(), "code-owned field "+field) {
				t.Fatalf("model metadata bypassed the sieve: %v", err)
			}
			after, _ := json.Marshal(ledger)
			if string(before) != string(after) {
				t.Fatal("rejected model metadata changed the input ledger")
			}
		})
	}
}

func handoffAmendment(id model.LedgerItemID, patch map[string]any) any {
	return map[string]any{"operations": []any{map[string]any{
		"kind": "amend", "target_id": id, "item": map[string]any{"task": patch}, "rationale": "owner clarified retained task",
	}}}
}

func handoffFixtureLedger(t *testing.T, name string) EngineeringLedger {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/planning-gauntlet/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Ledger EngineeringLedger `json:"base_ledger"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture.Ledger
}

func handoffController(t *testing.T, response string, semantic func(Assignment) any) *Controller {
	t.Helper()
	controller, _ := fixtureController(t, fixtureWorkload(), func(call Assignment) (any, error) {
		if call.Role != Plan {
			t.Fatalf("unexpected native role: %s", call.Role)
		}
		return response, nil
	}, passingChecks)
	leaf := sieveTestProvider(t, semantic)
	controller.Providers["parser"] = providerFunc(func(ctx context.Context, call Assignment, event func(NativeEvent) error) (Result, error) {
		return RunParserSieve(ctx, call, event, leaf)
	})
	return controller
}

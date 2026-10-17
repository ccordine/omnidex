package codingagent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestAddedPlanningItemsCannotSupplyTheirOwnReviewAuthority(t *testing.T) {
	items := []model.LedgerMutationItem{
		{Objective: &model.LedgerObjective{}}, {Requirement: &model.LedgerRequirement{}},
		{Constraint: &model.LedgerConstraint{}}, {Group: &model.LedgerGroup{}},
		{Task: &model.LedgerTask{}}, {AcceptanceCriterion: &model.LedgerAcceptanceCriterion{}},
		{Check: &model.LedgerCheck{}}, {Dependency: &model.LedgerDependency{}},
	}
	for _, item := range items {
		for _, state := range []model.LedgerReviewState{"", model.LedgerReviewAgreed} {
			base := item.Base()
			base.ID, base.ReviewState, base.Disposition = "new-item", state, model.LedgerDispositionTombstoned
			raw, _ := json.Marshal(map[string]any{"operations": []any{map[string]any{"kind": "add", "item": item, "rationale": "owner proposal"}}})
			operations, err := decodePlanningOperations(raw, planningLedgerFixture(model.LedgerReviewAgreed))
			if err != nil || len(operations) != 1 {
				t.Fatalf("decode addition: %s %v", raw, err)
			}
			parsed := operations[0].Item.Base()
			if parsed.ID != "new-item" || parsed.ReviewState != model.LedgerReviewProposed || parsed.Disposition != model.LedgerDispositionActive {
				t.Fatalf("model supplied lifecycle authority: %+v", parsed)
			}
		}
	}
}

func TestConstraintBindingFailureReturnsToParserWithoutReplanning(t *testing.T) {
	for _, rule := range []string{"Archive outputs stay in the chosen directory.", "Currency calculations use the selected exchange rate."} {
		t.Run(rule, func(t *testing.T) {
			ledger := planningLedgerFixture(model.LedgerReviewAgreed)
			before, _ := json.Marshal(ledger)
			plans, parses := 0, 0
			controller, _ := fixtureController(t, fixtureWorkload(), func(call Assignment) (any, error) {
				if call.Role == Plan {
					plans++
					return "Add project-wide constraint new-rule: " + rule, nil
				}
				if call.Role != PlanningMutation {
					t.Fatalf("unexpected role %s", call.Role)
				}
				parses++
				constraint := model.LedgerConstraint{LedgerItem: planningLedgerItem("new-rule", 5, ""), Rule: rule}
				operations := []any{map[string]any{"kind": "add", "item": model.LedgerMutationItem{Constraint: &constraint}, "rationale": "new owner constraint"}}
				if parses > 1 {
					if len(call.ValidationNotes) != 1 || !strings.Contains(call.ValidationNotes[0], "new-rule has no remaining task consumer") {
						t.Fatalf("retry lost exact structural defect: %+v", call.ValidationNotes)
					}
					operations = append(operations, map[string]any{"kind": "amend", "target_id": "task", "item": map[string]any{"task": map[string]any{"constraint_ids": []string{"new-rule"}}}, "rationale": "bind the stated project constraint"})
				}
				raw, _ := json.Marshal(map[string]any{"operations": operations})
				materialized, err := decodePlanningOperations(raw, ledger)
				return planningSieveMutationResult{Operations: materialized}, err
			}, passingChecks)
			turn := PlanningTurn{OperationID: "add-rule", OwnerTurnID: "owner-rule", ObservedRevision: ledger.Revision, Text: rule}
			var failure *ParserFailure
			if _, err := controller.PlanPlanningMutation(context.Background(), ledger, turn); !errors.As(err, &failure) {
				t.Fatalf("unconsumed constraint accepted: %v", err)
			}
			mutation, err := controller.PlanPlanningMutation(context.Background(), ledger, turn)
			if err != nil {
				t.Fatal(err)
			}
			next, err := ApplyPlanningMutation(ledger, mutation)
			if err != nil {
				t.Fatal(err)
			}
			if next.Constraints[0].ReviewState != model.LedgerReviewProposed || !reflect.DeepEqual(next.Tasks[0].ConstraintIDs, []model.LedgerItemID{"new-rule"}) || next.Tasks[0].Goal != ledger.Tasks[0].Goal {
				t.Fatalf("constraint did not become reviewable task guidance: %+v", next)
			}
			after, _ := json.Marshal(ledger)
			if string(before) != string(after) || plans != 1 || parses != 2 {
				t.Fatalf("parser recovery changed source state or replayed planning: plans=%d parses=%d", plans, parses)
			}
		})
	}
}

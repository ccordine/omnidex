package codingagent

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestPlanningCorrectionRemovesRestrictionWithQuotedReplacement(t *testing.T) {
	for _, name := range []string{"expense", "wordcounter"} {
		t.Run(name, func(t *testing.T) {
			ledger := handoffFixtureLedger(t, name)
			wanted := ledger.Constraints[0].Rule
			ledger.Constraints[0].Rule += " This planning session prohibits all future file changes."
			before, _ := json.Marshal(ledger)
			id := ledger.Constraints[0].ID
			response := fmt.Sprintf("Update only `%s` by removing its final sentence. The revised constraint is:\n\n> %s\n\nKeep the existing tasks, acceptance criteria and checks unchanged.", id, wanted)
			projections := 0
			controller := handoffController(t, response, func(call Assignment) any {
				step := call.ParserStep
				if step.Stage == "scan" {
					kind := "change"
					if strings.HasPrefix(step.Source, "Keep the existing") {
						kind = "ignore"
					}
					return scanOracle(step.Source, kind)
				}
				projections++
				if projections != 1 {
					t.Fatal("exact already-applied replacement was sent for another semantic edit")
				}
				return constraintAmendment(id, wanted)
			})
			turn := PlanningTurn{OperationID: "remove-restriction", OwnerTurnID: "owner", Text: "Remove only the temporary session restriction; preserve the rest.", ObservedRevision: ledger.Revision}
			mutation, err := controller.PlanPlanningMutation(t.Context(), ledger, turn)
			if err != nil {
				t.Fatal(err)
			}
			if projections != 1 || len(mutation.Operations) != 1 || mutation.Operations[0].TargetID != id {
				t.Fatalf("correction did not retain exactly its intended amendment: %+v", mutation)
			}
			next, err := ApplyPlanningMutation(ledger, mutation)
			if err != nil {
				t.Fatal(err)
			}
			var expected EngineeringLedger
			if err := json.Unmarshal(before, &expected); err != nil {
				t.Fatal(err)
			}
			expected.Revision++
			expected.Constraints[0].Rule = wanted
			expected.Constraints[0].ReviewState = model.LedgerReviewProposed
			if !reflect.DeepEqual(next, expected) {
				t.Fatalf("correction changed unrelated items, identity, provenance, rank or relationships: %+v", next)
			}
			after, _ := json.Marshal(ledger)
			if string(before) != string(after) {
				t.Fatal("preview changed the authoritative input ledger")
			}
			entries, err := controller.Evidence.Read(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			represented := false
			for _, entry := range entries {
				var frame struct {
					Stage string `json:"stage"`
					Value struct {
						Text   string             `json:"text"`
						ItemID model.LedgerItemID `json:"item_id"`
					} `json:"value"`
				}
				if json.Unmarshal(entry.Frame, &frame) == nil && frame.Stage == "planning_restatement" {
					represented = frame.Value.ItemID == id && strings.TrimSpace(frame.Value.Text) == "> "+wanted
				}
			}
			if !represented {
				t.Fatal("quoted source text lost its code-owned disposition evidence")
			}
		})
	}
}

func TestPlanningRestatementDoesNotHideUnrepresentedEdits(t *testing.T) {
	ledger := handoffFixtureLedger(t, "expense")
	wanted := ledger.Constraints[0].Rule
	ledger.Constraints[0].Rule += " Unwanted temporary restriction."
	before, _ := json.Marshal(ledger)
	id := ledger.Constraints[0].ID
	for _, unhandled := range []string{
		"> " + wanted + " Also erase the audit history.",
		"> " + ledger.Constraints[0].Rule,
		"Remove another requirement as well.",
	} {
		response := "Remove the temporary restriction.\n\n" + unhandled
		controller := handoffController(t, response, func(call Assignment) any {
			if call.ParserStep.Stage == "scan" {
				return scanOracle(call.ParserStep.Source, "change")
			}
			if strings.HasPrefix(call.ParserStep.Source, "Remove the temporary") {
				return constraintAmendment(id, wanted)
			}
			return map[string]any{"operations": []any{}}
		})
		turn := PlanningTurn{OperationID: "unhandled", OwnerTurnID: "owner", Text: response, ObservedRevision: ledger.Revision}
		if _, err := controller.PlanPlanningMutation(t.Context(), ledger, turn); err == nil || !strings.Contains(err.Error(), "produced no ledger effect") {
			t.Fatalf("unrepresented edit was silently accepted: %q: %v", unhandled, err)
		}
		after, _ := json.Marshal(ledger)
		if string(before) != string(after) {
			t.Fatal("rejected correction modified authoritative state")
		}
	}
	operation := model.LedgerMutationOperation{Kind: model.LedgerMutationAmend,
		Item: &model.LedgerMutationItem{Constraint: &ledger.Constraints[0]}}
	if representedPlanningStatement(wanted, ledger, nil) != "" || representedPlanningStatement(wanted, ledger, changedPlanningTexts(ledger, operation)) != "" {
		t.Fatal("unapplied replacement was accepted as represented")
	}
	changed := ledger.Constraints[0]
	changed.Rule = wanted
	operation.Item.Constraint = &changed
	proof := changedPlanningTexts(ledger, operation)
	if len(proof) != 1 || representedPlanningStatement(wanted, ledger, proof) != "" {
		t.Fatal("an earlier result that does not match current state supplied restatement evidence")
	}
	ledger.Constraints[0] = changed
	ledger.Constraints[0].Disposition = model.LedgerDispositionTombstoned
	if representedPlanningStatement(wanted, ledger, proof) != "" {
		t.Fatal("removed item supplied active restatement evidence")
	}
}

func constraintAmendment(id model.LedgerItemID, rule string) any {
	return map[string]any{"operations": []any{map[string]any{
		"kind": "amend", "target_id": id,
		"item":      map[string]any{"constraint": map[string]any{"rule": rule}},
		"rationale": "Remove only the unwanted restriction.",
	}}}
}

func TestPlanningRestatementAcceptsOnlyCompleteSentenceSpans(t *testing.T) {
	value := "Keep files local. Preserve original bytes. Ask before deleting files."
	for _, text := range []string{"Keep files local.", "Preserve original bytes.", "Preserve original bytes. Ask before deleting files.", value} {
		if !planningTextRepresentsStatement(value, text) {
			t.Fatalf("complete represented sentence span rejected: %q", text)
		}
	}
	for _, text := range []string{"files local.", "Preserve original", "Preserve original bytes. Delete the originals."} {
		if planningTextRepresentsStatement(value, text) {
			t.Fatalf("partial or new obligation treated as represented: %q", text)
		}
	}
}

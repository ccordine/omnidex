package codingagent

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestCapturedPlanningAmendmentsRetainKnownTaskState(t *testing.T) {
	for _, name := range []string{"expense", "wordcounter"} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile("../../testdata/planning-gauntlet/" + name + ".json")
			if err != nil {
				t.Fatal(err)
			}
			var fixture struct {
				Ledger   EngineeringLedger `json:"base_ledger"`
				Rejected json.RawMessage   `json:"mutation_parser_response"`
			}
			if err := json.Unmarshal(raw, &fixture); err != nil {
				t.Fatal(err)
			}
			ledger := fixture.Ledger
			before, _ := json.Marshal(ledger)
			// Historical malformed output remains invalid; no field-stripping bypass.
			operations, err := decodePlanningOperations(fixture.Rejected, ledger)
			if err == nil {
				mutation := LedgerMutation{OperationID: "replay", OwnerTurnID: "owner", BaseRevision: ledger.Revision,
					Operations: operations, OwnerTurnDisposition: parsedPlanningDisposition(ledger, operations)}
				if _, err := ApplyPlanningMutation(ledger, mutation); err == nil {
					t.Fatal("historical invalid parser payload was silently accepted")
				}
			}
			task := ledger.Tasks[0]
			patch, _ := json.Marshal(map[string]any{"operations": []any{map[string]any{
				"kind": "amend", "target_id": task.ID, "item": map[string]any{"task": map[string]any{"goal": nil, "requirement_ids": nil, "rationale": "updated planning note"}}, "rationale": "owner clarified the plan",
			}}})
			operations, err = decodePlanningOperations(patch, ledger)
			if err != nil {
				t.Fatal(err)
			}
			changed := *operations[0].Item.Task
			if changed.Rationale != "updated planning note" {
				t.Fatal("explicit field amendment lost")
			}
			changed.Rationale = task.Rationale
			if !reflect.DeepEqual(changed, task) {
				t.Fatalf("patch reconstructed accepted fields: %+v", changed)
			}
			after, _ := json.Marshal(ledger)
			if string(before) != string(after) {
				t.Fatal("parsing mutated the authoritative input ledger")
			}
		})
	}
}

func TestCapturedHistoryParserFailuresCannotMutateTheLedger(t *testing.T) {
	for _, name := range []string{"expense", "wordcounter"} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile("../../testdata/planning-gauntlet/" + name + ".json")
			if err != nil {
				t.Fatal(err)
			}
			var fixture struct {
				Ledger  EngineeringLedger `json:"base_ledger"`
				Failure struct {
					Response json.RawMessage `json:"parser_response"`
				} `json:"history_injection_parser_failure"`
			}
			if err := json.Unmarshal(raw, &fixture); err != nil {
				t.Fatal(err)
			}
			before, _ := json.Marshal(fixture.Ledger)
			operations, err := decodePlanningOperations(fixture.Failure.Response, fixture.Ledger)
			if err != nil {
				t.Fatal(err)
			}
			if len(operations) == 0 {
				t.Fatal("captured invalid operations disappeared")
			}
			mutation := LedgerMutation{OperationID: "captured-failure", OwnerTurnID: "owner", BaseRevision: fixture.Ledger.Revision,
				Operations: operations, OwnerTurnDisposition: parsedPlanningDisposition(fixture.Ledger, operations)}
			if _, err := ApplyPlanningMutation(fixture.Ledger, mutation); err == nil {
				t.Fatal("invented parser identities bypassed ledger validation")
			}
			after, _ := json.Marshal(fixture.Ledger)
			if string(before) != string(after) {
				t.Fatal("failed parser mutation changed accepted state")
			}
		})
	}
}

func TestPlanningAmendmentRejectsUnknownIdentityAndExplicitInvalidClears(t *testing.T) {
	ledger := planningLedgerFixture(model.LedgerReviewAgreed)
	for _, item := range []string{`{"goal":""}`, `{"id":"other"}`, `{"provenance":null}`, `{"not_a_field":null}`, `{"goal":17}`} {
		content := json.RawMessage(`{"operations":[{"kind":"amend","target_id":"task","item":{"task":` + item + `},"rationale":"change"}]}`)
		operations, err := decodePlanningOperations(content, ledger)
		if err == nil {
			mutation := LedgerMutation{OperationID: "op", OwnerTurnID: "owner", BaseRevision: ledger.Revision, Operations: operations,
				OwnerTurnDisposition: parsedPlanningDisposition(ledger, operations)}
			_, err = ApplyPlanningMutation(ledger, mutation)
		}
		if err == nil {
			t.Fatalf("invalid amendment accepted: %s", item)
		}
	}
	content := json.RawMessage(`{"operations":[{"kind":"amend","target_id":"task","item":{"task":{"read_files":[]}},"rationale":"clear"}]}`)
	ledger.Tasks[0].ReadFiles = []string{"existing.txt"}
	operations, err := decodePlanningOperations(content, ledger)
	if err != nil || len(operations[0].Item.Task.ReadFiles) != 0 {
		t.Fatalf("explicit clear was treated as unchanged: %+v %v", operations, err)
	}
}

func TestPlanningParserRepeatsHaveOneEffectWithoutWeakeningDomainValidation(t *testing.T) {
	ledger := planningLedgerFixture(model.LedgerReviewAgreed)
	operation := `{"kind":"amend","target_id":"task","item":{"task":{"goal":"clarified goal"}},"rationale":"owner clarification"}`
	operations, err := decodePlanningOperations(json.RawMessage(`{"operations":[`+operation+`,`+operation+`]}`), ledger)
	if err != nil || len(operations) != 1 {
		t.Fatalf("exact parser repetition was not normalized: %+v %v", operations, err)
	}
	mutation := LedgerMutation{OperationID: "op", OwnerTurnID: "owner", BaseRevision: ledger.Revision, Operations: operations,
		OwnerTurnDisposition: parsedPlanningDisposition(ledger, operations)}
	next, err := ApplyPlanningMutation(ledger, mutation)
	if err != nil || next.Tasks[0].Goal != "clarified goal" {
		t.Fatalf("one exact delta failed: %v", err)
	}
	mutation.Operations = append(mutation.Operations, mutation.Operations[0])
	if err := mutation.Validate(); err == nil {
		t.Fatal("authoritative domain mutation accepted duplicate operations")
	}
}

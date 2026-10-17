package codingagent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
)

func TestCapturedInvalidPlanningMutationRetriesParserNotPlanner(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/planning-gauntlet/preservation.json")
	if err != nil {
		t.Fatal(err)
	}
	var capture struct {
		Ledger    EngineeringLedger `json:"base_ledger"`
		Rejected  json.RawMessage   `json:"mutation_parser_response"`
		Responses []struct {
			OwnerTurn string `json:"owner_turn"`
			Response  string `json:"response"`
		} `json:"responses"`
	}
	if err := json.Unmarshal(raw, &capture); err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(capture.Ledger)
	plans, parses := 0, 0
	controller, _ := fixtureController(t, fixtureWorkload(), func(call Assignment) (any, error) {
		switch call.Role {
		case Plan:
			plans++
			return capture.Responses[1].Response, nil
		case PlanningMutation:
			parses++
			if call.PlanningResponse != capture.Responses[1].Response {
				t.Fatal("retry lost exact saved prose")
			}
			if parses == 1 {
				return capture.Rejected, nil
			}
			obligation := capture.Ledger.Tasks[1].DeliveryObligation + "\n" + capture.Responses[1].Response
			patch, _ := json.Marshal(map[string]any{"operations": []any{map[string]any{"kind": "amend", "target_id": "t2", "item": map[string]any{"task": map[string]any{"delivery_obligation": obligation}}, "rationale": "Add the stated baseline verification context."}}})
			operations, err := decodePlanningOperations(patch, capture.Ledger)
			return planningSieveMutationResult{Operations: operations}, err
		default:
			t.Fatalf("unexpected role %s", call.Role)
			return nil, nil
		}
	}, passingChecks)
	turn := PlanningTurn{OperationID: "captured-baselines", OwnerTurnID: "captured-owner", Text: capture.Responses[1].OwnerTurn, ObservedRevision: capture.Ledger.Revision}
	_, err = controller.PlanPlanningMutation(context.Background(), capture.Ledger, turn)
	var failure *ParserFailure
	if !errors.As(err, &failure) || !failure.Retryable() {
		t.Fatalf("invalid parser payload terminalized durable planning: %v", err)
	}
	after, _ := json.Marshal(capture.Ledger)
	if string(before) != string(after) {
		t.Fatal("invalid parser payload changed the ledger")
	}
	mutation, err := controller.PlanPlanningMutation(context.Background(), capture.Ledger, turn)
	if err != nil {
		t.Fatal(err)
	}
	next, err := ApplyPlanningMutation(capture.Ledger, mutation)
	if err != nil {
		t.Fatal(err)
	}
	next.Tasks[1].DeliveryObligation = capture.Ledger.Tasks[1].DeliveryObligation
	next.Revision = capture.Ledger.Revision
	if !reflect.DeepEqual(next, capture.Ledger) {
		t.Fatal("recovered parse stomped unchanged ledger state")
	}
	if plans != 1 || parses != 2 {
		t.Fatalf("retry repeated native planning: plans=%d parses=%d", plans, parses)
	}
}

package codingagent

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestPlanningParserKeepsAcceptedContractsOutOfMutableContext(t *testing.T) {
	done := packetFixture()
	done.ID, done.Goal, done.Objective = "done", "ACCEPTED_GOAL_PRIVATE", "ACCEPTED_OBJECTIVE_PRIVATE"
	done.Disposition = model.LedgerDispositionActive
	pending := packetFixture()
	pending.ID, pending.Goal, pending.Objective = "pending", "Remaining invoice search", "Filter saved invoices."
	pending.Disposition = model.LedgerDispositionActive
	ledger := EngineeringLedger{ID: "ledger", Revision: 4, Tasks: []LedgerTask{done, pending}}
	controller := Controller{CompletedTaskIDs: []string{"done"}}
	call := controller.planningAssignment(PlanningMutation, "mutation", "Extract changes.",
		PlanningTurn{OperationID: "retained", Text: "Keep completed work.", ObservedRevision: 4}, &ledger)
	if !reflect.DeepEqual(call.CompletedTaskIDs, []string{"done"}) || len(call.PlanningLedger.Tasks) != 2 {
		t.Fatal("completion identity or code-owned full ledger was lost")
	}
	raw, err := json.Marshal(PromptContext(call))
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{done.Goal, done.Objective} {
		if strings.Contains(string(raw), text) {
			t.Fatalf("accepted contract leaked through parser context: %s", text)
		}
	}
	for _, text := range []string{pending.Objective, `"completed_task_ids":["done"]`} {
		if !strings.Contains(string(raw), text) {
			t.Fatalf("planning context lost relevant state: %s", text)
		}
	}
	if ledger.Tasks[0].Goal != done.Goal || ledger.Tasks[1].Goal != pending.Goal {
		t.Fatal("context projection mutated authoritative instructions")
	}
}

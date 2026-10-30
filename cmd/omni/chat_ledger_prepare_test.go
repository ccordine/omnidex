package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

func TestFrozenLedgerManualSaveOpensWithoutPlannerMessage(t *testing.T) {
	snapshot := panelLedgerFixture()
	snapshot.State = "frozen"
	opened, saved := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/jobs/801/plan/item-planner":
			var request struct {
				OperationID string `json:"operation_id"`
				Manual      bool   `json:"manual"`
				ItemID      string `json:"item_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			if !request.Manual || request.ItemID != "panel-first" {
				t.Error("manual edit was routed to an AI planner")
			}
			opened++
			snapshot.Generation = 2
			snapshot.State = "proposed"
			snapshot.BaseRevision = 3
			snapshot.Revision = 4
			snapshot.Ledger.Revision = 4
			writeChatPlanReviewJSON(t, w, 200, map[string]any{"job_id": 801, "operation_id": request.OperationID, "status": "running"})
		case "/v1/jobs/801/plan":
			writeChatPlanReviewJSON(t, w, 200, snapshot)
		case "/v1/jobs/801/plan/decisions":
			request := decodePanelControl(t, r)
			if request.Revision != 4 {
				t.Error("manual save used the old frozen revision")
			}
			saved++
			snapshot.BaseRevision = 4
			snapshot.Revision = 5
			snapshot.Ledger.Revision = 5
			snapshot.OperationID = request.OperationID
			snapshot.Ledger.Tasks[0].Goal = request.Action.Item.Goal
			writeChatPlanReviewJSON(t, w, 200, snapshot)
		default:
			t.Error("unexpected manual editing request", r.URL.Path)
		}
	}))
	defer server.Close()
	session := panelSession(t, server.URL)
	view := session.ledgerWorkspace()
	if err := view.acceptLedger(snapshot); err != nil {
		t.Fatal(err)
	}
	task := snapshot.Ledger.Tasks[0]
	task.Goal = "Manually revised name"
	err := session.acceptLedgerPanelRequest(workspaceLedgerRequest{key: 'e', jobID: 801, generation: 1, revision: 3, action: client.DirectPlanningAction{Kind: model.LedgerMutationAmend, TargetID: task.ID, Item: &task}})
	if err != nil {
		t.Fatal(err)
	}
	if opened != 1 || saved != 1 || view.ledger.snapshot.Revision != 5 || view.ledger.snapshot.Ledger.Tasks[0].Goal != task.Goal {
		t.Fatal("manual save did not reopen and persist exactly once")
	}
}

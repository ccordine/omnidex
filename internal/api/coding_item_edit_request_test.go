package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestManualLedgerItemRequestAcceptsCanonicalTaskFields(t *testing.T) {
	task := model.LedgerTask{LedgerItem: model.LedgerItem{ID: "existing", Rank: 7, Provenance: model.LedgerProvenance{Kind: "owner_turn", Reference: "owner", Revision: 1}, Disposition: model.LedgerDispositionActive, ReviewState: model.LedgerReviewProposed}, Goal: "Edit this item", Objective: "Keep its task-specific instructions", OwnerContext: []model.TaskContext{{Text: "Relevant context", Provenance: model.LedgerProvenance{Kind: "owner_turn", Reference: "owner", Revision: 1}}}}
	raw, err := json.Marshal(map[string]any{"operation_id": "manual", "owner_turn_id": "owner", "owner_turn_text": "Save manual edit", "revision": 1, "workspace_root": "/workspace", "workspace_identity": "fixture", "action": codingagent.DirectPlanningAction{Kind: model.LedgerMutationAmend, TargetID: task.ID, Item: &task}})
	if err != nil {
		t.Fatal(err)
	}
	var request codingPlanDecisionsRequest
	err = decodeLifecycleControlObject(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/jobs/1/plan/decisions", strings.NewReader(string(raw))), "coding plan decisions", &request)
	if err != nil {
		t.Fatal(err)
	}
	if request.Action.Item.ID != task.ID || request.Action.Item.Objective != task.Objective || request.Action.Item.OwnerContext[0].Text != "Relevant context" {
		t.Fatal("manual request lost embedded or task-specific fields")
	}
}

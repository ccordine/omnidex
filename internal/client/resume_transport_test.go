package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestResumeRetainsObservedFrozenAuthority(t *testing.T) {
	channel := testCLIChannel("/fixture/workspace")
	id := testOperationID(t, "resume")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requireRequestAuthority(t, r, http.MethodPost, "/v1/jobs/73/resume", "")
		requireJSONBody(t, r, map[string]any{"operation_id": string(id), "feedback": "Continue retained work.", "expected_generation": float64(2), "observed_ledger_revision": float64(7), "workspace_root": channel.WorkspaceRoot, "workspace_identity": testWorkspaceIdentity})
		writeJSON(t, w, http.StatusOK, lifecycleControlReceipt{JobID: 73, OperationID: id, Status: model.JobStatusRunning})
	}))
	defer server.Close()
	client := testClient(t, server.URL)
	job, err := client.Resume(context.Background(), channel, testWorkspaceIdentity, 73, id, "Continue retained work.", 2, 7)
	if err != nil || job.ID != 73 || job.Status != model.JobStatusRunning {
		t.Fatalf("resume %+v %v", job, err)
	}
	if _, err := client.Resume(context.Background(), channel, testWorkspaceIdentity, 73, id, "Continue retained work.", 0, 7); err == nil {
		t.Fatal("missing generation admitted")
	}
}

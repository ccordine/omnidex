package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestCLIControlsUseExactJobBoundEndpointsAndPayloads(t *testing.T) {
	t.Parallel()

	workspaceRoot := "/tmp/reading list"
	channel := testCLIChannel(workspaceRoot)
	jobID := int64(73)
	tests := []struct {
		name      string
		action    string
		textField string
		text      string
		status    string
		invoke    func(*Client, queue.LifecycleOperationID) (model.Job, error)
	}{
		{
			name: "feedback", action: "feedback", textField: "feedback",
			text: "Use this clarification for the waiting job.", status: model.JobStatusRunning,
			invoke: func(apiClient *Client, operationID queue.LifecycleOperationID) (model.Job, error) {
				return apiClient.SubmitFeedback(
					context.Background(), channel, testWorkspaceIdentity, jobID, operationID,
					"Use this clarification for the waiting job.",
				)
			},
		},
		{
			name: "interrupt", action: "interrupt", textField: "feedback",
			text: "Pause while I inspect the current result.", status: model.JobStatusWaiting,
			invoke: func(apiClient *Client, operationID queue.LifecycleOperationID) (model.Job, error) {
				return apiClient.Interrupt(
					context.Background(), channel, testWorkspaceIdentity, jobID, operationID,
					"Pause while I inspect the current result.",
				)
			},
		},
		{
			name: "redirect", action: "replan", textField: "feedback",
			text: "Preserve the current headings and shorten each entry.", status: model.JobStatusRunning,
			invoke: func(apiClient *Client, operationID queue.LifecycleOperationID) (model.Job, error) {
				return apiClient.Replan(
					context.Background(), channel, testWorkspaceIdentity, jobID, operationID,
					"Preserve the current headings and shorten each entry.",
				)
			},
		},
		{
			name: "cancel", action: "cancel", textField: "reason",
			text: "Stop this objective.", status: model.JobStatusCanceled,
			invoke: func(apiClient *Client, operationID queue.LifecycleOperationID) (model.Job, error) {
				return apiClient.Cancel(
					context.Background(), channel, testWorkspaceIdentity, jobID, operationID,
					"Stop this objective.",
				)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			operationID := testOperationID(t, test.name+"-control")
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				requireRequestAuthority(
					t, request, http.MethodPost,
					fmt.Sprintf("/v1/jobs/%d/%s", jobID, test.action), "",
				)
				requireJSONBody(t, request, map[string]any{
					"operation_id":       string(operationID),
					test.textField:       test.text,
					"workspace_root":     workspaceRoot,
					"workspace_identity": testWorkspaceIdentity,
				})
				writeJSON(t, writer, http.StatusOK, lifecycleControlReceipt{
					JobID: jobID, OperationID: operationID, Status: test.status,
				})
			}))
			defer server.Close()

			job, err := test.invoke(testClient(t, server.URL), operationID)
			if err != nil {
				t.Fatalf("submit control: %v", err)
			}
			if job.ID != jobID || job.Status != test.status {
				t.Fatalf("control result = %#v, want job %d status %q", job, jobID, test.status)
			}
		})
	}
}

func TestCLIControlRejectsChangedJobIdentity(t *testing.T) {
	t.Parallel()

	channel := testCLIChannel("/tmp/work log")
	operationID := testOperationID(t, "changed-job")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requireRequestAuthority(t, request, http.MethodPost, "/v1/jobs/81/interrupt", "")
		writeJSON(t, writer, http.StatusOK, lifecycleControlReceipt{
			JobID: 82, OperationID: operationID, Status: model.JobStatusWaiting,
		})
	}))
	defer server.Close()

	_, err := testClient(t, server.URL).Interrupt(
		context.Background(), channel, testWorkspaceIdentity, 81, operationID,
		"Pause this objective.",
	)
	if err == nil || !strings.Contains(err.Error(), "does not match the exact submitted job operation") {
		t.Fatalf("changed job receipt error = %v", err)
	}
}

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestSessionTurnRejectsInvalidCodingModeBeforeRequest(t *testing.T) {
	t.Parallel()
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		http.Error(writer, "unexpected request", http.StatusInternalServerError)
	}))
	defer server.Close()
	apiClient := testClient(t, server.URL)
	for _, modes := range [][]codingagent.Mode{{"unknown"}, {codingagent.Agents, codingagent.Agents}} {
		if _, err := apiClient.SubmitSessionTurn(
			context.Background(), testCLIChannel("/workspace/mode-validation"), testWorkspaceIdentity,
			testOperationID(t, "mode-validation"), "Exact user request.", modes...,
		); err == nil {
			t.Errorf("unsupported coding mode selection %v was accepted", modes)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("invalid coding mode made %d HTTP requests", requests.Load())
	}
}

func TestSessionTurnCarriesExplicitAgentsMode(t *testing.T) {
	t.Parallel()
	channel := testCLIChannel("/workspace/explicit-mode")
	operationID := testOperationID(t, "explicit-agents")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requireJSONBody(t, request, map[string]any{
			"workspace_root": channel.WorkspaceRoot, "workspace_identity": testWorkspaceIdentity,
			"operation_id": string(operationID), "text": "Exact user request.", "coding_mode": "agents",
		})
		writeJSON(t, writer, http.StatusAccepted, SessionTurnReceipt{
			OperationID: operationID, Disposition: model.ChannelSessionTurnFeedback,
			ChannelID: channel.ID, WorkspaceRoot: channel.WorkspaceRoot,
			WorkspaceIdentity: testWorkspaceIdentity, JobID: 41, Generation: 1, Status: model.JobStatusRunning,
		})
	}))
	defer server.Close()
	if _, err := testClient(t, server.URL).SubmitSessionTurn(
		context.Background(), channel, testWorkspaceIdentity, operationID, "Exact user request.", codingagent.Agents,
	); err != nil {
		t.Fatalf("submit explicit Agents turn: %v", err)
	}
}

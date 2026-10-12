package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestTerminalTurnSelectsAgentsAndPreservesModeOnRetry(t *testing.T) {
	t.Parallel()
	var operations []model.LifecycleOperationID
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var payload struct {
			OperationID model.LifecycleOperationID `json:"operation_id"`
			Text        string                     `json:"text"`
			CodingMode  codingagent.Mode           `json:"coding_mode"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode terminal turn: %v", err)
		}
		if payload.CodingMode != codingagent.Agents || payload.Text != "Keep this exact turn." {
			t.Errorf("terminal turn mode=%q text=%q; want agents and exact text", payload.CodingMode, payload.Text)
		}
		operations = append(operations, payload.OperationID)
		http.Error(writer, "outcome remains unknown", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	session := newChatOperationTestSession(t, server.URL)
	if err := session.acceptText("Keep this exact turn."); !client.IsHTTPStatus(err, http.StatusServiceUnavailable) {
		t.Fatalf("initial ambiguous submit: %v", err)
	}
	if _, err := session.resolvePendingOperation(); !client.IsHTTPStatus(err, http.StatusServiceUnavailable) {
		t.Fatalf("replay ambiguous submit: %v", err)
	}
	if len(operations) != 2 || operations[0] == "" || operations[0] != operations[1] {
		t.Fatalf("retry changed the pending operation: %v", operations)
	}
}

func TestTerminalRetryUsesModeFrozenInPendingTurn(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var payload struct {
			CodingMode codingagent.Mode `json:"coding_mode"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode pending turn: %v", err)
		}
		if payload.CodingMode != codingagent.Normal {
			t.Errorf("retry recomputed mode %q instead of retained normal", payload.CodingMode)
		}
		http.Error(writer, "outcome remains unknown", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	session := newChatOperationTestSession(t, server.URL)
	operationID, err := newOperationID()
	if err != nil {
		t.Fatal(err)
	}
	session.pendingTurn = &pendingSessionTurn{
		exactText: "Retain the pending choice.", operationID: operationID, codingMode: codingagent.Normal,
	}
	if err := session.acceptText(session.pendingTurn.exactText); !client.IsHTTPStatus(err, http.StatusServiceUnavailable) {
		t.Fatalf("resubmit frozen pending turn: %v", err)
	}
	if _, err := session.resolvePendingOperation(); !client.IsHTTPStatus(err, http.StatusServiceUnavailable) {
		t.Fatalf("resolve frozen pending turn: %v", err)
	}
}

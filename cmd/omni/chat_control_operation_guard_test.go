package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"golang.org/x/term"
)

func TestControlFailureClassificationControlsGuard(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     int
		drop       bool
		definitive bool
	}{
		{name: "request timeout", status: http.StatusRequestTimeout},
		{name: "too early", status: http.StatusTooEarly},
		{name: "too many requests", status: http.StatusTooManyRequests},
		{name: "unexpected client response", status: http.StatusTeapot},
		{name: "conflict", status: http.StatusConflict, definitive: true},
		{name: "dropped transport", drop: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var session *chatSession
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodGet {
					writeOperationGuardJSON(t, writer, operationGuardActiveSnapshot(t, session))
					return
				}
				if request.Method != http.MethodPost || request.URL.Path != "/v1/jobs/41/interrupt" {
					t.Errorf("unexpected control request %s %s", request.Method, request.URL.Path)
					return
				}
				if test.drop {
					dropOperationGuardConnection(t, writer)
					return
				}
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(test.status)
				_, _ = writer.Write([]byte(`{"error":"control was not accepted"}`))
			}))
			defer server.Close()

			session = newChatOperationTestSession(t, server.URL)
			session.renderer = operationGuardRenderer()
			const exactText = "pause for exact inspection"
			_, err := session.control("interrupt", exactText, true, nil)
			if test.drop {
				if err == nil || client.IsDefinitiveMutationRejection(err) {
					t.Fatalf("dropped transport error = %v, want ambiguous outcome", err)
				}
			} else if !client.IsHTTPStatus(err, test.status) {
				t.Fatalf("control error = %v, want HTTP %d", err, test.status)
			}
			if test.definitive {
				if session.pendingControl != nil {
					t.Fatalf("definitive rejection retained pending control %#v", session.pendingControl)
				}
				return
			}
			if session.pendingControl == nil {
				t.Fatal("ambiguous response cleared pending control")
			}
			pendingID := session.pendingControl.operationID
			replayID, replayErr := session.controlOperationID(41, "interrupt", exactText, true)
			if replayErr != nil || replayID != pendingID {
				t.Fatalf("replayed operation = %q, error %v; want %q", replayID, replayErr, pendingID)
			}
			if _, differentErr := session.controlOperationID(41, "cancel", "cancel instead", true); differentErr == nil {
				t.Fatal("different control bypassed ambiguous operation guard")
			}
		})
	}
}

func operationGuardActiveSnapshot(t *testing.T, session *chatSession) client.ChatSessionSnapshot {
	t.Helper()
	if session == nil {
		t.Fatal("operation guard session is unavailable")
	}
	now := time.Now().UTC()
	const instruction = "keep the objective active"
	metadata, err := json.Marshal(map[string]any{
		"channel_id": session.channel.ID, "channel_user_message_id": int64(1),
		"client_cwd":                session.channel.WorkspaceRoot,
		"client_workspace_identity": session.workspaceIdentity,
	})
	if err != nil {
		t.Fatalf("encode active job metadata: %v", err)
	}
	return client.ChatSessionSnapshot{
		Revision: "channel_session_revision_" + strings.Repeat("b", 64),
		Channel:  session.channel, WorkspaceIdentity: session.workspaceIdentity,
		Messages: []model.ChannelMessage{{
			ID: 1, ChannelID: session.channel.ID, Role: model.ChannelMessageRoleUser,
			Content: instruction, CreatedAt: now,
			Turn: &model.ChannelMessageTurnState{JobID: 41, Status: model.JobStatusRunning, UpdatedAt: now},
		}},
		Turns: []queue.ChannelSessionTurn{}, Controls: []queue.ChannelSessionControl{},
		ActiveJob: &model.JobDetails{
			Job: model.Job{
				ID: 41, Instruction: instruction, Pipeline: model.PipelineChat,
				Status: model.JobStatusRunning, Metadata: metadata, CurrentGeneration: 1,
				CreatedAt: now, UpdatedAt: now,
			},
			Steps: []model.Step{{
				ID: 1, JobID: 41, Action: "test", Status: model.StepStatusRunning,
				Generation: 1, CreatedAt: now, UpdatedAt: now,
			}},
		},
	}
}

func operationGuardRenderer() chatRenderer {
	var output bytes.Buffer
	terminal := term.NewTerminal(
		terminalReadWriter{reader: strings.NewReader(""), writer: &output},
		"you> ",
	)
	return chatRenderer{console: &chatConsole{
		terminal: terminal, terminalFD: -1, prompt: "you> ",
	}}
}

func writeOperationGuardJSON(t *testing.T, writer http.ResponseWriter, value any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		t.Errorf("encode operation guard response: %v", err)
	}
}

func dropOperationGuardConnection(t *testing.T, writer http.ResponseWriter) {
	t.Helper()
	hijacker, ok := writer.(http.Hijacker)
	if !ok {
		t.Fatal("test server does not support connection hijacking")
	}
	connection, _, err := hijacker.Hijack()
	if err != nil {
		t.Fatalf("hijack control response: %v", err)
	}
	_ = connection.Close()
}

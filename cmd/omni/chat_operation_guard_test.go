package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestWhitespaceInterruptUsesCanonicalDefaultReason(t *testing.T) {
	t.Parallel()

	actual, err := validatedChatControlText("interrupt", " \t ")
	if err != nil {
		t.Fatalf("validate whitespace interrupt: %v", err)
	}
	if actual != ctrlCInterruptReason {
		t.Fatalf("whitespace interrupt reason = %q, want %q", actual, ctrlCInterruptReason)
	}
}

func TestOverLimitChatInputDoesNotReservePendingOperation(t *testing.T) {
	t.Parallel()

	overLimit := strings.Repeat("x", model.MaxFreeFormTurnBytes+1)
	session := &chatSession{}
	if _, err := session.sessionTurnOperationID(overLimit); err == nil {
		t.Fatal("over-limit ordinary turn unexpectedly reserved an operation")
	}
	if session.pendingTurn != nil {
		t.Fatalf("over-limit ordinary turn retained pending operation %#v", session.pendingTurn)
	}
	for _, action := range []string{"interrupt", "redirect"} {
		t.Run(action, func(t *testing.T) {
			localSession := &chatSession{}
			if _, err := localSession.control(action, overLimit, true, nil); err == nil {
				t.Fatalf("over-limit /%s unexpectedly reserved an operation", action)
			}
			if localSession.pendingControl != nil {
				t.Fatalf("over-limit /%s retained pending operation %#v", action, localSession.pendingControl)
			}
		})
	}
}

func TestTurnHTTPFailureClassificationControlsGuard(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     int
		definitive bool
	}{
		{name: "request timeout", status: http.StatusRequestTimeout},
		{name: "too early", status: http.StatusTooEarly},
		{name: "too many requests", status: http.StatusTooManyRequests},
		{name: "unexpected client response", status: http.StatusTeapot},
		{name: "conflict", status: http.StatusConflict, definitive: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(test.status)
				_, _ = writer.Write([]byte(`{"error":"mutation was not accepted"}`))
			}))
			defer server.Close()

			session := newChatOperationTestSession(t, server.URL)
			const exactText = "preserve this exact turn identity"
			err := session.acceptText(exactText)
			if !client.IsHTTPStatus(err, test.status) {
				t.Fatalf("turn error = %v, want HTTP %d", err, test.status)
			}
			if test.definitive {
				if session.pendingTurn != nil {
					t.Fatalf("definitive rejection retained pending turn %#v", session.pendingTurn)
				}
				return
			}
			if session.pendingTurn == nil {
				t.Fatal("ambiguous HTTP response cleared pending turn")
			}
			pendingID := session.pendingTurn.operationID
			replayID, replayErr := session.sessionTurnOperationID(exactText)
			if replayErr != nil || replayID != pendingID {
				t.Fatalf("replayed operation = %q, error %v; want %q", replayID, replayErr, pendingID)
			}
			if _, differentErr := session.sessionTurnOperationID("different turn"); differentErr == nil {
				t.Fatal("different turn bypassed ambiguous operation guard")
			}
		})
	}
}

func TestAmbiguousTurnFailureRetainsExactGuard(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		hijacker, ok := writer.(http.Hijacker)
		if !ok {
			t.Error("test server does not support connection hijacking")
			return
		}
		connection, _, err := hijacker.Hijack()
		if err != nil {
			t.Errorf("hijack ambiguous response: %v", err)
			return
		}
		_ = connection.Close()
	}))
	defer server.Close()

	session := newChatOperationTestSession(t, server.URL)
	const exactText = "retain this operation until its outcome is known"
	if err := session.acceptText(exactText); err == nil || definitiveChatRequestFailure(err) {
		t.Fatalf("ambiguous turn error = %v, want non-definitive transport failure", err)
	}
	if session.pendingTurn == nil {
		t.Fatal("ambiguous failure cleared its pending operation")
	}
	pendingID := session.pendingTurn.operationID
	replayID, err := session.sessionTurnOperationID(exactText)
	if err != nil || replayID != pendingID {
		t.Fatalf("same ambiguous turn = operation %q error %v, want retained %q", replayID, err, pendingID)
	}
	if _, err := session.sessionTurnOperationID("a different turn"); err == nil {
		t.Fatal("different turn bypassed ambiguous pending operation")
	}
	if session.pendingTurn == nil || session.pendingTurn.operationID != pendingID {
		t.Fatalf("different turn changed ambiguous operation %#v", session.pendingTurn)
	}
}

func newChatOperationTestSession(t *testing.T, baseURL string) *chatSession {
	t.Helper()
	apiClient, err := client.New(baseURL, 2*time.Second)
	if err != nil {
		t.Fatalf("create test client: %v", err)
	}
	now := time.Now().UTC()
	return &chatSession{
		ctx:    context.Background(),
		client: apiClient,
		channel: model.Channel{
			ID:            model.ChannelID("cli-chat-" + strings.Repeat("d", 64)),
			Scope:         model.ChannelScopeUser,
			Name:          "CLI operation guard test",
			Tags:          []string{"chat", "cli"},
			WorkspaceRoot: "/tmp/cli-operation-guard-test",
			Mode:          model.ChannelModeAssistant,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		workspaceIdentity: "directory_identity_v1_" + strings.Repeat("a", 64),
		signals:           make(chan os.Signal),
		messages:          make(map[int64]model.ChannelMessage),
		turns:             make(map[queue.LifecycleOperationID]queue.ChannelSessionTurn),
		controls:          make(map[queue.LifecycleOperationID]queue.ChannelSessionControl),
	}
}

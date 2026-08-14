package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/queue"
)

func TestDecodeAIControlRequestAcceptsOnlyCanonicalActions(t *testing.T) {
	for _, action := range []aiControlAction{aiControlActionPause, aiControlActionResume} {
		req := httptest.NewRequest(http.MethodPost, "/v1/ai/control", strings.NewReader(`{"action":"`+string(action)+`"}`))
		response := httptest.NewRecorder()
		decoded, err := decodeAIControlRequest(response, req)
		if err != nil {
			t.Fatalf("decode %q: %v", action, err)
		}
		if decoded.Action != action {
			t.Fatalf("action=%q want %q", decoded.Action, action)
		}
	}
}

func TestDecodeAIControlRequestRejectsLooseOrUnboundedAuthority(t *testing.T) {
	invalidUTF8 := append([]byte(`{"action":"`), 0xff)
	invalidUTF8 = append(invalidUTF8, []byte(`"}`)...)
	tests := []struct {
		name string
		body []byte
	}{
		{name: "duplicate", body: []byte(`{"action":"pause","action":"resume"}`)},
		{name: "unknown", body: []byte(`{"action":"pause","paused":true}`)},
		{name: "inexact case field", body: []byte(`{"Action":"pause"}`)},
		{name: "trailing document", body: []byte(`{"action":"pause"} {}`)},
		{name: "null action", body: []byte(`{"action":null}`)},
		{name: "missing action", body: []byte(`{}`)},
		{name: "action synonym", body: []byte(`{"action":"play"}`)},
		{name: "inexact action case", body: []byte(`{"action":"PAUSE"}`)},
		{name: "padded action", body: []byte(`{"action":" pause "}`)},
		{name: "invalid UTF-8", body: invalidUTF8},
		{name: "oversized transport", body: []byte(`{"action":"` + strings.Repeat("x", int(maxAIControlRequestBodyBytes)) + `"}`)},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/v1/ai/control", bytes.NewReader(testCase.body))
			response := httptest.NewRecorder()
			if _, err := decodeAIControlRequest(response, request); err == nil {
				t.Fatalf("invalid AI control body accepted: %q", testCase.body)
			}
		})
	}
}

func TestAIControlHasNoLooseActionCompatibility(t *testing.T) {
	source := readAPISource(t, "ai_control.go")
	for _, forbidden := range []string{
		`Paused *bool`,
		`case "resume", "play"`,
		`strings.ToLower(req.Action)`,
		`json.NewDecoder(r.Body).Decode(&req)`,
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("AI control retains loose action authority %q", forbidden)
		}
	}
	browser := readFrontendSource(t, "web/src/controllers/shell_controller.ts")
	if !strings.Contains(browser, `jsonRequest({ action })`) {
		t.Fatal("AI control browser must send the one canonical action DTO")
	}
	if strings.Contains(browser, `jsonRequest({ paused:`) {
		t.Fatal("AI control browser must not send the removed dual paused field")
	}
}

func TestAIControlRejectsQueryAuthorityBeforeStateAccess(t *testing.T) {
	server := NewServer(&queue.Repository{}, nil)
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		request := httptest.NewRequest(method, "/v1/ai/control?action=pause", strings.NewReader(`{"action":"pause"}`))
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("method=%s status=%d body=%s", method, response.Code, response.Body.String())
		}
	}
}

func TestPublishAIControlUpdateUsesTypedRealtimeState(t *testing.T) {
	hub := NewRealtimeHub()
	subscription, err := hub.Subscribe([]string{realtimeTopicUI}, 0)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer subscription.Unsubscribe()
	server := &Server{realtimeHub: hub}
	state := aiControlState{
		Paused:    true,
		Counts:    map[string]int64{"pending": 3, "running": 0, "waiting_input": 0},
		UpdatedAt: time.Now().UTC(),
	}
	if err := server.publishAIControlUpdate(state, "paused"); err != nil {
		t.Fatalf("publish: %v", err)
	}
	select {
	case raw := <-subscription.Messages:
		var message realtimeMessage
		if err := json.Unmarshal(raw, &message); err != nil {
			t.Fatalf("decode event: %v", err)
		}
		if message.EventName != "ai-control-updated" || message.AIControl == nil || !message.AIControl.Paused {
			t.Fatalf("unexpected event: %#v", message)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for AI control event")
	}
}

func TestPublishAIControlUpdateFailsWithoutRealtimeHub(t *testing.T) {
	if err := (&Server{}).publishAIControlUpdate(aiControlState{}, "resume"); err == nil {
		t.Fatal("expected unavailable realtime hub to fail")
	}
}

func TestAIControlCommittedPostEffectFailureUsesTypedPartialCommitStatus(t *testing.T) {
	if status := aiControlMutationHTTPStatus(aiControlResponse{CommitState: aiControlCommitted}); status != http.StatusOK {
		t.Fatalf("complete mutation status=%d", status)
	}
	for _, response := range []aiControlResponse{
		{CommitState: aiControlCommittedDegraded, OperationError: "Scrum reconciliation failed"},
		{CommitState: aiControlCommittedDegraded, RealtimeError: "publication failed"},
	} {
		if status := aiControlMutationHTTPStatus(response); status != http.StatusMultiStatus {
			t.Fatalf("degraded mutation status=%d response=%+v", status, response)
		}
	}
}

func TestAIControlPostCommitStateReadFailureReturnsTypedDegradedAuthority(t *testing.T) {
	t.Parallel()
	server := &Server{repo: &queue.Repository{}}
	response, err := server.aiControlMutationResponse(
		t.Context(), 3, false, true, "paused",
	)
	if err != nil {
		t.Fatalf("post-commit read failure escaped as an ordinary error: %v", err)
	}
	if response.CommitState != aiControlCommittedDegraded || !response.Paused ||
		response.CanceledJobs != 3 || response.OperationError == "" {
		t.Fatalf("response=%+v", response)
	}
	if response.Counts != nil {
		t.Fatalf("unread counts were synthesized: %+v", response.Counts)
	}
	if status := aiControlMutationHTTPStatus(response); status != http.StatusMultiStatus {
		t.Fatalf("status=%d want %d", status, http.StatusMultiStatus)
	}
}

package omni

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/agentstream"
)

type fixedExternalAgentSession struct {
	events []agentstream.Event
}

func (s fixedExternalAgentSession) Start(context.Context, ExternalAgentJob) (<-chan agentstream.Event, error) {
	events := make(chan agentstream.Event, len(s.events))
	for _, event := range s.events {
		events <- event
	}
	close(events)
	return events, nil
}

func (fixedExternalAgentSession) Cancel(context.Context, string) error { return nil }
func (fixedExternalAgentSession) Cleanup(context.Context) error        { return nil }

func TestStreamExternalAgentSessionRejectsUnknownTypeBeforeCallback(t *testing.T) {
	callbackCount := 0
	_, err := StreamExternalAgentSession(t.Context(), fixedExternalAgentSession{events: []agentstream.Event{{
		Agent: "codex", Type: agentstream.EventType("tool_call"), Message: "must not persist",
	}}}, ExternalAgentJob{SessionID: "session-1", Agent: "codex"}, func(agentstream.Event) error {
		callbackCount++
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("error=%v want unsupported type", err)
	}
	if callbackCount != 0 {
		t.Fatalf("invalid event reached persistence callback %d times", callbackCount)
	}
}

func TestStreamExternalAgentSessionRejectsInvalidSequenceBeforeCallback(t *testing.T) {
	seen := make([]agentstream.EventType, 0, 2)
	_, err := StreamExternalAgentSession(t.Context(), fixedExternalAgentSession{events: []agentstream.Event{
		{Agent: "codex", Type: agentstream.EventStarted, Message: "started"},
		{Agent: "codex", Type: agentstream.EventCompleted, Message: "done"},
		{Agent: "codex", Type: agentstream.EventMessage, Message: "after completion must not persist"},
	}}, ExternalAgentJob{SessionID: "session-1", Agent: "codex"}, func(event agentstream.Event) error {
		seen = append(seen, event.Type)
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "after its completed event") {
		t.Fatalf("error=%v want post-completion rejection", err)
	}
	if len(seen) != 2 || seen[0] != agentstream.EventStarted || seen[1] != agentstream.EventCompleted {
		t.Fatalf("invalid sequence event reached persistence callback: %v", seen)
	}
}

func TestStreamExternalAgentSessionPreservesOpaqueContentBeforeCallback(t *testing.T) {
	want := "  {not-json}\n\ttool-looking  "
	session := fixedExternalAgentSession{events: []agentstream.Event{
		{Agent: "codex", Type: agentstream.EventStarted, Message: "started"},
		{Agent: "codex", Type: agentstream.EventMessage, Message: want},
		{Agent: "codex", Type: agentstream.EventCompleted, Message: "done"},
	}}
	seen := ""
	_, err := StreamExternalAgentSession(t.Context(), session, ExternalAgentJob{
		SessionID: "session-1", Agent: "codex",
	}, func(event agentstream.Event) error {
		if event.Type == agentstream.EventMessage {
			seen = event.Message
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen != want {
		t.Fatalf("callback content=%q want exact %q", seen, want)
	}
}

func TestStreamExternalAgentSessionDoesNotInferFailureFromRawStatus(t *testing.T) {
	session := fixedExternalAgentSession{events: []agentstream.Event{
		{Agent: "cursor", Type: agentstream.EventStarted, Message: "started"},
		{Agent: "cursor", Type: agentstream.EventStatus, Message: `{"status":"ERROR"}`, Raw: json.RawMessage(`{"status":"ERROR"}`)},
		{Agent: "cursor", Type: agentstream.EventCompleted, Message: "typed completion"},
	}}
	result, err := StreamExternalAgentSession(t.Context(), session, ExternalAgentJob{
		SessionID: "session-1", Agent: "cursor",
	}, nil)
	if err != nil {
		t.Fatalf("raw provider prose must not override typed event lifecycle: %v", err)
	}
	if result.Summary != "typed completion" {
		t.Fatalf("result=%+v", result)
	}
}

func TestScanExternalAgentJSONLRejectsUnknownTypeWithoutEmission(t *testing.T) {
	events := make(chan agentstream.Event, 1)
	err := scanExternalAgentJSONL(t.Context(), strings.NewReader(
		`{"agent":"codex","type":"tool_call","message":"bad"}`+"\n",
	), ExternalAgentJob{SessionID: "session-1", Agent: "codex"}, events)
	if err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("error=%v want unsupported type", err)
	}
	if len(events) != 0 {
		t.Fatalf("invalid boundary event was emitted: %+v", <-events)
	}
}

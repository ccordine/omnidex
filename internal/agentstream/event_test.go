package agentstream

import (
	"strings"
	"testing"
)

func TestEventLineRoundTripPreservesOpaqueContentExactly(t *testing.T) {
	want := "  {not-json: [tool-looking]}\n\t  "
	line, err := EncodeLine(Event{
		SessionID: "session-1",
		Agent:     "codex",
		Type:      EventMessage,
		Message:   want,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if got.Message != want {
		t.Fatalf("content=%q want exact %q", got.Message, want)
	}
}

func TestDecodeEventLineRejectsMalformedUnknownAndUnboundPayloads(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{name: "malformed", line: `{not-json}`, want: "decode"},
		{name: "unknown field", line: `{"session_id":"s","agent":"codex","type":"message","message":"ok","invented":true}`, want: "unknown field"},
		{name: "duplicate field", line: `{"session_id":"s","agent":"codex","type":"message","type":"tool","message":"ok"}`, want: "duplicate key"},
		{name: "trailing value", line: `{"session_id":"s","agent":"codex","type":"message","message":"ok"} {}`, want: "trailing"},
		{name: "unknown type", line: `{"session_id":"s","agent":"codex","type":"tool_call","message":"ok"}`, want: "unsupported type"},
		{name: "missing session", line: `{"agent":"codex","type":"message","message":"ok"}`, want: "session_id"},
		{name: "empty content", line: `{"session_id":"s","agent":"codex","type":"message"}`, want: "message"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeLine(test.line)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v want %q", err, test.want)
			}
		})
	}
}

func TestEveryRegisteredEventTypeRoundTrips(t *testing.T) {
	for _, eventType := range []EventType{
		EventStarted, EventStatus, EventMessage, EventThinking, EventCommand,
		EventTool, EventFileChange, EventCompleted, EventError, EventInterrupted,
	} {
		t.Run(string(eventType), func(t *testing.T) {
			line, err := EncodeLine(Event{SessionID: "s", Agent: "codex", Type: eventType, Message: " exact "})
			if err != nil {
				t.Fatal(err)
			}
			event, err := DecodeLine(line)
			if err != nil {
				t.Fatal(err)
			}
			if event.Type != eventType || event.Message != " exact " {
				t.Fatalf("event=%+v", event)
			}
		})
	}
}

func TestBoundaryEventAllowsCodeToBindMissingSession(t *testing.T) {
	event, err := DecodeBoundaryLine(`{"agent":"cursor","type":"status","message":"  running  "}`)
	if err != nil {
		t.Fatal(err)
	}
	if event.SessionID != "" || event.Message != "  running  " {
		t.Fatalf("event=%+v", event)
	}
	if _, err := EncodeBoundaryLine(event); err != nil {
		t.Fatal(err)
	}
}

func TestDecodeBoundaryEventRejectsInvalidUTF8AndOversize(t *testing.T) {
	invalid := string([]byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'})
	if _, err := DecodeBoundaryLine(invalid); err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("invalid UTF-8 error=%v", err)
	}
	oversized := strings.Repeat("x", MaxEventBytes+1)
	if _, err := DecodeBoundaryLine(oversized); err == nil || !strings.Contains(err.Error(), "maximum") {
		t.Fatalf("oversize error=%v", err)
	}
}

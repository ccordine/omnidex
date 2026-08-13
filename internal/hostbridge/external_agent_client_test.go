package hostbridge

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/agentstream"
)

func TestReadExternalAgentEventsUsesStrictSharedEnvelope(t *testing.T) {
	want := "  {not-json}\n tool-looking  "
	line, err := agentstream.EncodeBoundaryLine(agentstream.Event{
		Agent: "codex", Type: agentstream.EventMessage, Message: want,
	})
	if err != nil {
		t.Fatal(err)
	}
	seen := ""
	err = ReadExternalAgentEvents(strings.NewReader(line+"\n"), func(event agentstream.Event) error {
		seen = event.Message
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen != want {
		t.Fatalf("content=%q want exact %q", seen, want)
	}
}

func TestReadExternalAgentEventsRejectsUnknownTypedPayload(t *testing.T) {
	called := false
	err := ReadExternalAgentEvents(strings.NewReader(
		`{"agent":"codex","type":"message","message":"ok","invented":true}`+"\n",
	), func(agentstream.Event) error {
		called = true
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error=%v want unknown-field failure", err)
	}
	if called {
		t.Fatal("invalid host event reached callback")
	}
}

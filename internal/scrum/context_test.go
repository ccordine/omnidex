package scrum

import (
	"encoding/json"
	"testing"
)

func TestIsScrumRawPlay(t *testing.T) {
	raw := json.RawMessage(`{"source":"omni-scrum","execution_agent":"cursor"}`)
	if !IsScrumRawPlay(raw) {
		t.Fatal("expected cursor scrum card to be raw play")
	}
	omni := json.RawMessage(`{"source":"omni-scrum","execution_agent":"omnidex"}`)
	if IsScrumRawPlay(omni) {
		t.Fatal("expected omnidex scrum card not to be raw play by default")
	}
}

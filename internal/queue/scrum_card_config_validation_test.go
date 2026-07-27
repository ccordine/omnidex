package queue

import (
	"encoding/json"
	"testing"
)

func TestCanonicalAgentConfigRejectsRemovedStrictToggle(t *testing.T) {
	if _, err := canonicalAgentConfig(json.RawMessage(`{"agent_strict":"true"}`)); err == nil {
		t.Fatal("expected removed agent_strict toggle to fail")
	}
}

func TestCanonicalAgentConfigPreservesValidatedValues(t *testing.T) {
	raw, err := canonicalAgentConfig(json.RawMessage(`{"agent_system":"codex","codex_model":"gpt-codex"}`))
	if err != nil {
		t.Fatalf("canonicalize agent config: %v", err)
	}
	if string(raw) != `{"agent_system":"codex","codex_model":"gpt-codex"}` &&
		string(raw) != `{"codex_model":"gpt-codex","agent_system":"codex"}` {
		t.Fatalf("unexpected canonical config: %s", raw)
	}
}

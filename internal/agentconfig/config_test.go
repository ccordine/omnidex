package agentconfig

import (
	"encoding/json"
	"testing"
)

func TestMergeAgentPriority(t *testing.T) {
	env := Config{"agent_system": "omnidex"}
	project := Config{"agent_system": "cursor"}
	card := Config{"agent_strict": "true"}
	merged := Merge(env, project, card)
	if merged.System() != SystemCursor {
		t.Fatalf("expected cursor, got %q", merged.System())
	}
	if !merged.IsStrict() {
		t.Fatal("expected strict=true")
	}
}

func TestFromSettingsJSON(t *testing.T) {
	raw := json.RawMessage(`{"agent_config":{"agent_system":"codex"}}`)
	cfg := FromSettingsJSON(raw)
	if cfg.System() != SystemCodex {
		t.Fatalf("expected codex, got %q", cfg.System())
	}
}

func TestNormalizeSystem(t *testing.T) {
	if normalizeSystem("local") != SystemOmnidex {
		t.Fatal("local should map to omnidex")
	}
	if normalizeSystem("cursor_sdk") != SystemCursor {
		t.Fatal("cursor_sdk should map to cursor")
	}
}

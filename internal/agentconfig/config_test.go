package agentconfig

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMergeAgentPriority(t *testing.T) {
	merged := Merge(Config{"agent_system": "omnidex"}, Config{"agent_system": "cursor"})
	if merged.System() != SystemCursor {
		t.Fatalf("expected cursor, got %q", merged.System())
	}
}

func TestFromSettingsJSON(t *testing.T) {
	cfg, err := FromSettingsJSON(json.RawMessage(`{"agent_config":{"agent_system":"codex"}}`))
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if cfg.System() != SystemCodex {
		t.Fatalf("expected codex, got %q", cfg.System())
	}
}

func TestFromJobMetadataRequiresAuthoritativeNestedConfig(t *testing.T) {
	cfg, err := FromJobMetadata(json.RawMessage(`{"source":"omni-scrum","agent_config":{"agent_system":"cursor"}}`))
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if cfg.System() != SystemCursor {
		t.Fatalf("expected cursor, got %q", cfg.System())
	}
}

func TestFromJobMetadataDefaultsUnconfiguredJobToOmnidex(t *testing.T) {
	cfg, err := FromJobMetadata(json.RawMessage(`{"source":"omni-web-chat"}`))
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if cfg.System() != SystemOmnidex {
		t.Fatalf("expected omnidex, got %q", cfg.System())
	}
}

func TestFromJobMetadataRejectsRemovedTopLevelAgentFields(t *testing.T) {
	for _, raw := range []string{
		`{"execution_agent":"cursor"}`,
		`{"agent_strict":true,"agent_config":{"agent_system":"cursor"}}`,
	} {
		if _, err := FromJobMetadata(json.RawMessage(raw)); err == nil {
			t.Fatalf("expected removed metadata field to fail: %s", raw)
		}
	}
}

func TestFromJSONRejectsInvalidState(t *testing.T) {
	cases := []string{
		`null`,
		`[]`,
		`{"agent_system":true}`,
		`{"agent_system":"local"}`,
		`{"agent_system":"bogus"}`,
		`{"agent_strict":"true"}`,
		`{"unknown":"value"}`,
		`{"codex_network_access":"yes"}`,
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			if _, err := FromJSON(json.RawMessage(raw)); err == nil {
				t.Fatalf("expected invalid config to fail: %s", raw)
			}
		})
	}
}

func TestFromSettingsJSONPropagatesMalformedNestedConfig(t *testing.T) {
	_, err := FromSettingsJSON(json.RawMessage(`{"agent_config":{"agent_system":7}}`))
	if err == nil || !strings.Contains(err.Error(), "must be a string") {
		t.Fatalf("expected typed failure, got %v", err)
	}
}

func TestCodexModelConfig(t *testing.T) {
	cfg, err := FromJSON(json.RawMessage(`{
		"agent_system":"codex",
		"codex_model":"gpt-codex-project",
		"codex_reasoning_effort":"high",
		"codex_sandbox_mode":"workspace-write",
		"codex_approval_policy":"never",
		"codex_network_access":"false",
		"codex_web_search_mode":"disabled"
	}`))
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if cfg.System() != SystemCodex || cfg.CodexModel() != "gpt-codex-project" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}

func TestCursorModelConfig(t *testing.T) {
	cfg, err := FromJSON(json.RawMessage(`{"agent_system":"cursor","cursor_model":"composer-test"}`))
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if cfg.System() != SystemCursor || cfg.CursorModel() != "composer-test" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}

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

func TestFromJobMetadataAcceptsTopLevelExecutionAgent(t *testing.T) {
	raw := json.RawMessage(`{"source":"omni-scrum","execution_agent":"cursor","agent_strict":true}`)
	cfg := FromJobMetadata(raw)
	if cfg.System() != SystemCursor {
		t.Fatalf("expected cursor, got %q", cfg.System())
	}
	if !cfg.IsStrict() {
		t.Fatal("expected top-level agent_strict=true")
	}
}

func TestFromJobMetadataAcceptsTopLevelCodexExecutionAgent(t *testing.T) {
	raw := json.RawMessage(`{"source":"omni-scrum","execution_agent":"codex"}`)
	cfg := FromJobMetadata(raw)
	if cfg.System() != SystemCodex {
		t.Fatalf("expected codex, got %q", cfg.System())
	}
}

func TestFromJobMetadataPreservesNestedConfigAndFillsMissingSystem(t *testing.T) {
	raw := json.RawMessage(`{"execution_agent":"cursor","agent_config":{"cursor_model":"composer-test"}}`)
	cfg := FromJobMetadata(raw)
	if cfg.System() != SystemCursor {
		t.Fatalf("expected cursor from top-level execution_agent, got %q", cfg.System())
	}
	if cfg.CursorModel() != "composer-test" {
		t.Fatalf("expected cursor model preserved, got %q", cfg.CursorModel())
	}
}

func TestCodexModelConfig(t *testing.T) {
	cfg := FromJSON(json.RawMessage(`{
		"agent_system":"codex",
		"codex_model":"gpt-codex-project",
		"codex_reasoning_effort":"high",
		"codex_sandbox_mode":"workspace-write",
		"codex_approval_policy":"never",
		"codex_network_access":"false",
		"codex_web_search_mode":"disabled"
	}`))
	if cfg.System() != SystemCodex {
		t.Fatalf("expected codex, got %q", cfg.System())
	}
	if cfg.CodexModel() != "gpt-codex-project" {
		t.Fatalf("expected codex model, got %q", cfg.CodexModel())
	}
	if cfg.CodexReasoningEffort() != "high" {
		t.Fatalf("expected reasoning effort, got %q", cfg.CodexReasoningEffort())
	}
	if cfg.CodexSandboxMode() != "workspace-write" {
		t.Fatalf("expected sandbox, got %q", cfg.CodexSandboxMode())
	}
	if cfg.CodexApprovalPolicy() != "never" {
		t.Fatalf("expected approval policy, got %q", cfg.CodexApprovalPolicy())
	}
	if cfg.CodexNetworkAccess() != "false" {
		t.Fatalf("expected network access, got %q", cfg.CodexNetworkAccess())
	}
	if cfg.CodexWebSearchMode() != "disabled" {
		t.Fatalf("expected web search mode, got %q", cfg.CodexWebSearchMode())
	}
}

func TestCursorModelConfig(t *testing.T) {
	cfg := FromJSON(json.RawMessage(`{"agent_system":"cursor","cursor_model":"composer-test"}`))
	if cfg.System() != SystemCursor {
		t.Fatalf("expected cursor, got %q", cfg.System())
	}
	if cfg.CursorModel() != "composer-test" {
		t.Fatalf("expected cursor model, got %q", cfg.CursorModel())
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

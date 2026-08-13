package main

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/agentconfig"
)

func TestCLIAgentRuntimeConfigFromFlagsAppliesCodexOverrides(t *testing.T) {
	cfg, err := cliAgentRuntimeConfigFromFlags(cliAgentRuntimeFlags{
		AgentSystem:          "codex",
		AgentModel:           "gpt-5.3-codex",
		CodexReasoningEffort: "high",
		CodexSandboxMode:     "workspace-write",
		CodexApprovalPolicy:  "on-request",
		CodexNetworkAccess:   "false",
		CodexWebSearchMode:   "disabled",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	metadata := map[string]any{}
	cfg.ApplyToMetadata(metadata)
	raw, ok := metadata["instance_agent_config"].(map[string]string)
	if !ok {
		t.Fatalf("expected instance_agent_config map, got %#v", metadata["instance_agent_config"])
	}
	for key, want := range map[string]string{
		"agent_system":           agentconfig.SystemCodex,
		"codex_model":            "gpt-5.3-codex",
		"codex_reasoning_effort": "high",
		"codex_sandbox_mode":     "workspace-write",
		"codex_approval_policy":  "on-request",
		"codex_network_access":   "false",
		"codex_web_search_mode":  "disabled",
	} {
		if raw[key] != want {
			t.Fatalf("%s=%q want %q in %#v", key, raw[key], want, raw)
		}
	}
}

func TestCLIAgentRuntimeConfigRejectsAgentModelWithoutAgent(t *testing.T) {
	_, err := cliAgentRuntimeConfigFromFlags(cliAgentRuntimeFlags{AgentModel: "composer-2"})
	if err == nil || !strings.Contains(err.Error(), "requires explicit --agent") {
		t.Fatalf("expected explicit active-agent error, got %v", err)
	}
}

func TestCLIAgentRuntimeConfigRejectsConflictingAgentModel(t *testing.T) {
	_, err := cliAgentRuntimeConfigFromFlags(cliAgentRuntimeFlags{
		AgentSystem: "cursor",
		AgentModel:  "composer-2",
		CursorModel: "composer-1",
	})
	if err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestCLIAgentRuntimeConfigRejectsRemovedStrictToggle(t *testing.T) {
	cfg := newCLIAgentRuntimeConfig()
	err := cfg.Set("agent_strict", "true")
	if err == nil || !strings.Contains(err.Error(), "unknown agent setting") {
		t.Fatalf("expected removed agent_strict to fail explicitly, got %v", err)
	}
}

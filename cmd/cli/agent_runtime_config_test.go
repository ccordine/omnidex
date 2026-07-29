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
	if err == nil || !strings.Contains(err.Error(), "run /agent") {
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

func TestSetActiveChatModelUsesExternalAgentModel(t *testing.T) {
	cfg := newCLIAgentRuntimeConfig()
	if err := cfg.Set("agent_system", "cursor"); err != nil {
		t.Fatalf("set agent: %v", err)
	}
	metadata := map[string]any{}
	msg, err := setActiveChatModel(metadata, cfg, "composer-2")
	if err != nil {
		t.Fatalf("set active model: %v", err)
	}
	if !strings.Contains(msg, "composer-2") {
		t.Fatalf("expected model in message, got %q", msg)
	}
	if got := cfg.ToMap()["cursor_model"]; got != "composer-2" {
		t.Fatalf("cursor_model=%q", got)
	}
}

func TestSetActiveChatModelUsesOmnidexRoleModels(t *testing.T) {
	cfg := newCLIAgentRuntimeConfig()
	if err := cfg.Set("agent_system", "omnidex"); err != nil {
		t.Fatalf("set agent: %v", err)
	}
	metadata := map[string]any{}
	_, err := setActiveChatModel(metadata, cfg, "qwen3:14b")
	if err != nil {
		t.Fatalf("set active model: %v", err)
	}
	for _, key := range []string{"model_plan", "model_analyze", "model_response", "model_verify"} {
		if metadata[key] != "qwen3:14b" {
			t.Fatalf("%s=%#v", key, metadata[key])
		}
	}
}

func TestSetChatMetadataOverrideOnlyAcceptsRuntimeBackedModels(t *testing.T) {
	metadata := map[string]any{}
	handled, err := setChatMetadataOverride(metadata, "model_plan", "qwen3:14b")
	if err != nil || !handled {
		t.Fatalf("expected model override handled, handled=%v err=%v", handled, err)
	}
	if metadata["model_plan"] != "qwen3:14b" {
		t.Fatalf("model_plan=%#v", metadata["model_plan"])
	}
	handled, err = setChatMetadataOverride(metadata, "reasoning", "deep")
	if err != nil || handled {
		t.Fatalf("write-only reasoning control must be unknown, handled=%v err=%v", handled, err)
	}
}

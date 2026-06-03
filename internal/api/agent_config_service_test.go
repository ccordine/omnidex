package api

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/agentconfig"
	"github.com/gryph/omnidex/internal/model"
)

func TestResolveAgentConfigPriority(t *testing.T) {
	s := &Server{}
	project := model.Project{
		Settings: json.RawMessage(`{"agent_config":{"agent_system":"cursor"}}`),
	}
	card := ScrumCard{
		AgentConfig: json.RawMessage(`{"agent_strict":"true"}`),
	}

	resolved, source := s.resolveAgentConfig(context.Background(), project, card)
	if source != "card" {
		t.Fatalf("expected card source, got %q", source)
	}
	if resolved.System() != agentconfig.SystemCursor {
		t.Fatalf("expected cursor, got %q", resolved.System())
	}
	if !resolved.IsStrict() {
		t.Fatal("expected strict=true")
	}
}

func TestAgentConfigJobMetadataExternal(t *testing.T) {
	s := &Server{}
	project := model.Project{
		Settings: json.RawMessage(`{"agent_config":{"agent_system":"cursor","cursor_model":"composer-project","agent_strict":"true"}}`),
	}
	payload := s.agentConfigJobMetadata(context.Background(), project, ScrumCard{})
	if payload["execution_agent"] != agentconfig.SystemCursor {
		t.Fatalf("expected cursor execution agent, got %#v", payload["execution_agent"])
	}
	if payload["agent_strict"] != true {
		t.Fatalf("expected strict flag, got %#v", payload["agent_strict"])
	}
	agents, ok := payload["external_agents_used"].([]string)
	if !ok || len(agents) != 1 || agents[0] != "cursor_sdk" {
		t.Fatalf("expected external_agents_used, got %#v", payload["external_agents_used"])
	}
	cfg, ok := payload["agent_config"].(map[string]string)
	if !ok {
		t.Fatalf("expected agent_config map, got %#v", payload["agent_config"])
	}
	if cfg["cursor_model"] != "composer-project" {
		t.Fatalf("expected cursor_model in job metadata, got %#v", cfg)
	}
}

func TestAgentConfigJobMetadataCodexExternal(t *testing.T) {
	s := &Server{}
	project := model.Project{
		Settings: json.RawMessage(`{"agent_config":{
			"agent_system":"codex",
			"codex_model":"gpt-codex-project",
			"codex_reasoning_effort":"high",
			"codex_sandbox_mode":"workspace-write",
			"codex_approval_policy":"never",
			"codex_network_access":"false",
			"codex_web_search_mode":"disabled"
		}}`),
	}
	payload := s.agentConfigJobMetadata(context.Background(), project, ScrumCard{})
	if payload["execution_agent"] != agentconfig.SystemCodex {
		t.Fatalf("expected codex execution agent, got %#v", payload["execution_agent"])
	}
	agents, ok := payload["external_agents_used"].([]string)
	if !ok || len(agents) != 1 || agents[0] != "codex_sdk" {
		t.Fatalf("expected codex external agent, got %#v", payload["external_agents_used"])
	}
	cfg, ok := payload["agent_config"].(map[string]string)
	if !ok {
		t.Fatalf("expected agent_config map, got %#v", payload["agent_config"])
	}
	for key, want := range map[string]string{
		"codex_model":            "gpt-codex-project",
		"codex_reasoning_effort": "high",
		"codex_sandbox_mode":     "workspace-write",
		"codex_approval_policy":  "never",
		"codex_network_access":   "false",
		"codex_web_search_mode":  "disabled",
	} {
		if cfg[key] != want {
			t.Fatalf("expected %s=%q, got %#v", key, want, cfg)
		}
	}
}

func TestResolveAgentConfigInstancePriority(t *testing.T) {
	s := &Server{}
	project := model.Project{
		Settings: json.RawMessage(`{"agent_config":{"agent_system":"cursor"}}`),
	}
	card := ScrumCard{
		AgentConfig: json.RawMessage(`{"agent_system":"codex"}`),
	}
	instance := agentconfig.Config{"agent_system": "cursor"}

	resolved, source := s.resolveAgentConfig(context.Background(), project, card, instance)
	if source != agentconfig.SourceInstance {
		t.Fatalf("expected instance source, got %q", source)
	}
	if resolved.System() != agentconfig.SystemCursor {
		t.Fatalf("expected cursor from instance, got %q", resolved.System())
	}
}

func TestMergeProjectAgentConfig(t *testing.T) {
	settings := json.RawMessage(`{"theme":"dark","model_config":{"default_model":"x"}}`)
	override := json.RawMessage(`{"agent_system":"cursor"}`)
	merged, err := mergeProjectAgentConfig(settings, override)
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(merged, &root); err != nil {
		t.Fatalf("unmarshal merged: %v", err)
	}
	if string(root["model_config"]) == "" {
		t.Fatal("expected model_config preserved")
	}
	if string(root["agent_config"]) != string(override) {
		t.Fatalf("expected agent_config preserved, got %s", string(root["agent_config"]))
	}
}

func TestAgentConfigPatchKeepsExplicitOmnidexOverride(t *testing.T) {
	raw, err := agentConfigPatchFromRequest(json.RawMessage(`{"agent_system":"omnidex"}`))
	if err != nil {
		t.Fatalf("patch failed: %v", err)
	}
	var values map[string]string
	if err := json.Unmarshal(raw, &values); err != nil {
		t.Fatalf("unmarshal patch: %v", err)
	}
	if values["agent_system"] != "omnidex" {
		t.Fatalf("expected explicit omnidex override, got %#v", values)
	}
}

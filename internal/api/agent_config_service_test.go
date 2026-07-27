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
		AgentConfig: json.RawMessage(`{"cursor_model":"composer-card"}`),
	}

	resolved, source, err := s.resolveAgentConfig(context.Background(), project, card)
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}
	if source != "card" {
		t.Fatalf("expected card source, got %q", source)
	}
	if resolved.System() != agentconfig.SystemCursor {
		t.Fatalf("expected cursor, got %q", resolved.System())
	}
	if resolved.CursorModel() != "composer-card" {
		t.Fatalf("expected card model, got %q", resolved.CursorModel())
	}
}

func TestAgentConfigJobMetadataExternal(t *testing.T) {
	s := &Server{}
	project := model.Project{
		Settings: json.RawMessage(`{"agent_config":{"agent_system":"cursor","cursor_model":"composer-project"}}`),
	}
	payload, err := s.agentConfigJobMetadata(context.Background(), project, ScrumCard{})
	if err != nil {
		t.Fatalf("build metadata: %v", err)
	}
	if _, ok := payload["execution_agent"]; ok {
		t.Fatalf("removed execution_agent must be absent: %#v", payload)
	}
	if _, ok := payload["agent_strict"]; ok {
		t.Fatalf("removed agent_strict must be absent: %#v", payload)
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
	payload, err := s.agentConfigJobMetadata(context.Background(), project, ScrumCard{})
	if err != nil {
		t.Fatalf("build metadata: %v", err)
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

	resolved, source, err := s.resolveAgentConfig(context.Background(), project, card, instance)
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}
	if source != agentconfig.SourceInstance {
		t.Fatalf("expected instance source, got %q", source)
	}
	if resolved.System() != agentconfig.SystemCursor {
		t.Fatalf("expected cursor from instance, got %q", resolved.System())
	}
}

func TestResolveAgentConfigRejectsMalformedProjectState(t *testing.T) {
	s := &Server{}
	project := model.Project{Settings: json.RawMessage(`{"agent_config":{"agent_system":true}}`)}
	if _, _, err := s.resolveAgentConfig(context.Background(), project, ScrumCard{}); err == nil {
		t.Fatal("expected malformed project agent configuration to fail")
	}
}

func TestAgentConfigPatchRejectsRemovedStrictToggle(t *testing.T) {
	if _, err := agentConfigPatchFromRequest(json.RawMessage(`{"agent_strict":"true"}`)); err == nil {
		t.Fatal("expected removed agent_strict toggle to fail")
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

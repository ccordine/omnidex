package api

import (
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

	resolved, source := s.resolveAgentConfig(project, card)
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
		Settings: json.RawMessage(`{"agent_config":{"agent_system":"codex","agent_strict":"true"}}`),
	}
	payload := s.agentConfigJobMetadata(project, ScrumCard{})
	if payload["execution_agent"] != agentconfig.SystemCodex {
		t.Fatalf("expected codex execution agent, got %#v", payload["execution_agent"])
	}
	if payload["agent_strict"] != true {
		t.Fatalf("expected strict flag, got %#v", payload["agent_strict"])
	}
	agents, ok := payload["external_agents_used"].([]string)
	if !ok || len(agents) != 1 || agents[0] != "codex_sdk" {
		t.Fatalf("expected external_agents_used, got %#v", payload["external_agents_used"])
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

package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestEnvModelConfigProcessEnvironmentOverridesEnvFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("OMNI_CONVERSATION_RESPONSE_MODEL=file-response\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OMNI_ENV_FILE", path)
	t.Setenv("OMNI_CONVERSATION_RESPONSE_MODEL", "process-response")

	cfg, err := (&Server{}).envModelConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Get("conversation_response_model") != "process-response" {
		t.Fatalf("process environment did not override env file: %#v", cfg)
	}
}

func TestEnvModelConfigRejectsRemovedRoutes(t *testing.T) {
	t.Run("file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), ".env")
		if err := os.WriteFile(path, []byte("OLLAMA_MODEL_PLANNER=removed\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("OMNI_ENV_FILE", path)
		if _, err := (&Server{}).envModelConfig(); err == nil {
			t.Fatal("removed file route must fail")
		}
	})
	t.Run("process", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), ".env")
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("OMNI_ENV_FILE", path)
		t.Setenv("OMNI_PLANNER_MODEL", "")
		if _, err := (&Server{}).envModelConfig(); err == nil {
			t.Fatal("removed process route must fail even when explicitly empty")
		}
	})
}

func TestResolveModelConfigPriority(t *testing.T) {
	s := &Server{}
	project := model.Project{
		Settings: json.RawMessage(`{"model_config":{"conversation_response_model":"project-response"}}`),
	}
	card := ScrumCard{
		ModelConfig: json.RawMessage(`{"coding_fragment_model":"card-fragment"}`),
	}

	resolved, source, err := s.resolveModelConfig(project, card)
	if err != nil {
		t.Fatal(err)
	}
	if source != "card" {
		t.Fatalf("expected card source, got %q", source)
	}
	if resolved.Get("conversation_response_model") != "project-response" {
		t.Fatalf("expected inherited project response route, got %q", resolved.Get("conversation_response_model"))
	}
	if resolved.Get("coding_fragment_model") != "card-fragment" {
		t.Fatalf("expected card fragment route, got %q", resolved.Get("coding_fragment_model"))
	}
}

func TestResolveModelConfigRejectsMalformedDurableLayers(t *testing.T) {
	s := &Server{}
	for _, project := range []model.Project{
		{Settings: json.RawMessage(`null`)},
		{Settings: json.RawMessage(`{"model_config":{"unknown_model":"x"}}`)},
		{Settings: json.RawMessage(`{"model_config":{"default_model":42}}`)},
	} {
		if _, _, err := s.resolveModelConfig(project, ScrumCard{}); err == nil {
			t.Fatalf("project settings %s must fail", project.Settings)
		}
	}
	if _, _, err := s.resolveModelConfig(model.Project{}, ScrumCard{ModelConfig: json.RawMessage(`{"coding_fragment_model":false}`)}); err == nil {
		t.Fatal("malformed card model config must fail")
	}
}

func TestEnrichJobMetadataSkipsWhenPresent(t *testing.T) {
	s := &Server{}
	raw := []byte(`{"model_config":{"conversation_response_model":"preset"},"project_id":1}`)
	out, pulled, err := s.enrichJobMetadata(context.Background(), raw, ScrumCard{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pulled) != 0 {
		t.Fatalf("expected no pulls, got %v", pulled)
	}
	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	cfg, ok := payload["model_config"].(map[string]any)
	if !ok {
		t.Fatalf("expected model_config map, got %#v", payload["model_config"])
	}
	if cfg["conversation_response_model"] != "preset" {
		t.Fatalf("expected preset model, got %#v", cfg["conversation_response_model"])
	}
}

func TestEnrichJobMetadataAppliesInstanceAgentConfigForExplicitCoding(t *testing.T) {
	s := &Server{}
	raw := []byte(`{
		"client_cwd":"/tmp/work",
		"instance_agent_config":{
			"agent_system":"codex",
			"codex_model":"gpt-5.3-codex",
			"codex_reasoning_effort":"high"
		}
	}`)
	out, _, err := s.enrichJobMetadata(context.Background(), raw, ScrumCard{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload["agent_config_source"] != "instance" {
		t.Fatalf("agent_config_source=%#v want instance", payload["agent_config_source"])
	}
	cfg, ok := payload["agent_config"].(map[string]any)
	if !ok {
		t.Fatalf("expected agent_config map, got %#v", payload["agent_config"])
	}
	if cfg["codex_model"] != "gpt-5.3-codex" || cfg["codex_reasoning_effort"] != "high" {
		t.Fatalf("agent_config not preserved: %#v", cfg)
	}
}

func TestMergeProjectModelConfig(t *testing.T) {
	settings := json.RawMessage(`{"theme":"dark"}`)
	override := json.RawMessage(`{"conversation_response_model":"project-model"}`)
	merged, err := mergeProjectModelConfig(settings, override)
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(merged, &root); err != nil {
		t.Fatalf("unmarshal merged: %v", err)
	}
	if string(root["model_config"]) != string(override) {
		t.Fatalf("expected model_config preserved, got %s", string(root["model_config"]))
	}
}

package api

import (
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

	resolved, source, err := s.resolveModelConfig(project)
	if err != nil {
		t.Fatal(err)
	}
	if source != "project" {
		t.Fatalf("expected project source, got %q", source)
	}
	if resolved.Get("conversation_response_model") != "project-response" {
		t.Fatalf("expected project response route, got %q", resolved.Get("conversation_response_model"))
	}
}

func TestResolveModelConfigRejectsMalformedDurableLayers(t *testing.T) {
	s := &Server{}
	for _, project := range []model.Project{
		{Settings: json.RawMessage(`null`)},
		{Settings: json.RawMessage(`{"model_config":{"unknown_model":"x"}}`)},
		{Settings: json.RawMessage(`{"model_config":{"default_model":42}}`)},
	} {
		if _, _, err := s.resolveModelConfig(project); err == nil {
			t.Fatalf("project settings %s must fail", project.Settings)
		}
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

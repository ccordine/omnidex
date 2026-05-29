package modelconfig

import (
	"encoding/json"
	"testing"
)

func TestMergePriority(t *testing.T) {
	env := Config{"default_model": "env-model", "planner_model": "env-planner"}
	project := Config{"default_model": "project-model"}
	card := Config{"planner_model": "card-planner"}

	merged := Merge(env, project, card)
	if merged.Get("default_model") != "project-model" {
		t.Fatalf("expected project default_model, got %q", merged.Get("default_model"))
	}
	if merged.Get("planner_model") != "card-planner" {
		t.Fatalf("expected card planner_model, got %q", merged.Get("planner_model"))
	}
}

func TestFromSettingsJSON(t *testing.T) {
	raw := json.RawMessage(`{"model_config":{"default_model":"project-only"}}`)
	cfg := FromSettingsJSON(raw)
	if cfg.Get("default_model") != "project-only" {
		t.Fatalf("expected project-only, got %q", cfg.Get("default_model"))
	}
}

func TestOllamaModelNamesSkipsCloudModels(t *testing.T) {
	cfg := Config{
		"default_model":  "llama3.2:latest",
		"planner_model":  "gpt-4o",
		"thinking_model": "qwen2.5:7b",
	}
	names := cfg.OllamaModelNames()
	if len(names) != 2 {
		t.Fatalf("expected 2 ollama models, got %v", names)
	}
}

func TestLooksLikeOllamaModel(t *testing.T) {
	if !looksLikeOllamaModel("llama3.2:latest") {
		t.Fatal("expected ollama model name")
	}
	if looksLikeOllamaModel("gpt-4o") {
		t.Fatal("expected cloud model to be rejected")
	}
}

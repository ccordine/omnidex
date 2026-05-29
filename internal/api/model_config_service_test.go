package api

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestResolveModelConfigPriority(t *testing.T) {
	s := &Server{}
	project := model.Project{
		Settings: json.RawMessage(`{"model_config":{"default_model":"project-model"}}`),
	}
	card := ScrumCard{
		ModelConfig: json.RawMessage(`{"planner_model":"card-planner"}`),
	}

	resolved, source := s.resolveModelConfig(project, card)
	if source != "card" {
		t.Fatalf("expected card source, got %q", source)
	}
	if resolved.Get("default_model") != "project-model" {
		t.Fatalf("expected inherited project default_model, got %q", resolved.Get("default_model"))
	}
	if resolved.Get("planner_model") != "card-planner" {
		t.Fatalf("expected card planner_model, got %q", resolved.Get("planner_model"))
	}
}

func TestEnrichJobMetadataSkipsWhenPresent(t *testing.T) {
	s := &Server{}
	raw := []byte(`{"model_config":{"default_model":"preset"},"project_id":1}`)
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
	if cfg["default_model"] != "preset" {
		t.Fatalf("expected preset model, got %#v", cfg["default_model"])
	}
}

func TestMergeProjectModelConfig(t *testing.T) {
	settings := json.RawMessage(`{"theme":"dark"}`)
	override := json.RawMessage(`{"default_model":"project-model"}`)
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

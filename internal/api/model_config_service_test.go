package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
)

func TestEnvModelConfigProcessEnvironmentOverridesEnvFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("OLLAMA_MODEL_REASONING=file-reasoning\nOLLAMA_MODEL_PLANNER=file-planner\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OMNI_ENV_FILE", path)
	t.Setenv("OMNI_REASONING_MODEL", "")
	t.Setenv("OMNI_PLANNER_MODEL", "")
	t.Setenv("OLLAMA_MODEL_REASONING", "process-reasoning")
	t.Setenv("OLLAMA_MODEL_PLANNER", "process-planner")

	cfg, err := (&Server{}).envModelConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Get("reasoning_model") != "process-reasoning" || cfg.Get("planner_model") != "process-planner" {
		t.Fatalf("process environment did not override env file: %#v", cfg)
	}
}

func TestResolveModelConfigPriority(t *testing.T) {
	s := &Server{}
	project := model.Project{
		Settings: json.RawMessage(`{"model_config":{"default_model":"project-model"}}`),
	}
	card := ScrumCard{
		ModelConfig: json.RawMessage(`{"planner_model":"card-planner"}`),
	}

	resolved, source, err := s.resolveModelConfig(project, card)
	if err != nil {
		t.Fatal(err)
	}
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
	if _, _, err := s.resolveModelConfig(model.Project{}, ScrumCard{ModelConfig: json.RawMessage(`{"planner_model":false}`)}); err == nil {
		t.Fatal("malformed card model config must fail")
	}
}

func TestEnsureOllamaModelsDoesNotPullCloudProviderModels(t *testing.T) {
	server := NewServerWithOptions(nil, &fakeLLMClient{}, ServerOptions{
		ProviderConfig: config.Config{
			LLMProvider:  "qwen",
			DefaultModel: "Qwen/Qwen3-32B",
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	pulled, err := server.ensureOllamaModels(ctx, modelconfig.Config{
		"default_model": "Qwen/Qwen3-32B",
	})
	if err != nil {
		t.Fatalf("remote provider model preparation attempted an Ollama request: %v", err)
	}
	if len(pulled) != 0 {
		t.Fatalf("remote provider unexpectedly pulled Ollama models: %v", pulled)
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

func TestEnrichJobMetadataGeneralWebChatUsesNativeAgentWithoutWorkspace(t *testing.T) {
	s := &Server{}
	raw := []byte(`{"source":"omni-web-chat"}`)
	out, _, err := s.enrichJobMetadata(context.Background(), raw, ScrumCard{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	cfg, ok := payload["agent_config"].(map[string]any)
	if !ok || cfg["agent_system"] != "omnidex" {
		t.Fatalf("agent_config=%#v want omnidex", payload["agent_config"])
	}
	if payload["agent_config_source"] != "general_chat" {
		t.Fatalf("agent_config_source=%#v want general_chat", payload["agent_config_source"])
	}
}

func TestEnrichJobMetadataAppliesInstanceAgentConfigForCLIChat(t *testing.T) {
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

func TestGeneralWebChatWithoutWorkspaceRequiresNoProjectContext(t *testing.T) {
	if !generalWebChatWithoutWorkspace(map[string]any{"source": "omni-web-chat"}) {
		t.Fatal("expected plain web chat to be workspace-free")
	}
	if generalWebChatWithoutWorkspace(map[string]any{"source": "omni-web-chat", "project_id": float64(42)}) {
		t.Fatal("project chat should keep project agent routing")
	}
	if generalWebChatWithoutWorkspace(map[string]any{"source": "omni-web-chat", "client_cwd": "/tmp/project"}) {
		t.Fatal("chat with cwd should keep workspace agent routing")
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

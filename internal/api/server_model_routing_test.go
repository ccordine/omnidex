package api

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/config"
)

func TestRequiredDefaultLLMModelUsesConfiguredProvider(t *testing.T) {
	server := NewServerWithOptions(nil, &fakeLLMClient{}, ServerOptions{
		ProviderConfig: config.Config{LLMProvider: "openai", DefaultModel: "gpt-test"},
	})

	modelName, err := server.requiredDefaultLLMModel()
	if err != nil {
		t.Fatalf("requiredDefaultLLMModel() error: %v", err)
	}
	if modelName != "gpt-test" {
		t.Fatalf("requiredDefaultLLMModel()=%q want gpt-test", modelName)
	}
}

func TestRequiredDefaultLLMModelRejectsMissingProviderModel(t *testing.T) {
	server := NewServerWithOptions(nil, &fakeLLMClient{}, ServerOptions{ProviderConfig: config.Config{LLMProvider: "google"}})

	_, err := server.requiredDefaultLLMModel()
	if err == nil || !strings.Contains(err.Error(), "google") {
		t.Fatalf("requiredDefaultLLMModel() error=%v, want google model failure", err)
	}
}

func TestResolvedScrumCoachConfigUsesAuthoritativeProviderModel(t *testing.T) {
	server := NewServerWithOptions(nil, &fakeLLMClient{}, ServerOptions{
		ProviderConfig: config.Config{LLMProvider: "openai", DefaultModel: "gpt-coach"},
	})
	cfg, err := server.scrumCoachConfig(nil)
	if err != nil {
		t.Fatalf("scrumCoachConfig() error: %v", err)
	}
	if cfg.Model != "gpt-coach" {
		t.Fatalf("scrumCoachConfig().Model=%q want gpt-coach", cfg.Model)
	}
}

func TestRequiredDefaultLLMModelUsesChineseProviderWithoutProviderSwitch(t *testing.T) {
	server := NewServerWithOptions(nil, &fakeLLMClient{}, ServerOptions{
		ProviderConfig: config.Config{
			LLMProvider:  "qwen",
			DefaultModel: "qwen-current",
			ProviderModels: map[string]config.ProviderModelConfig{
				"qwen": {Default: "qwen-current"},
			},
		},
	})

	modelName, err := server.requiredDefaultLLMModel()
	if err != nil {
		t.Fatalf("requiredDefaultLLMModel() error: %v", err)
	}
	if modelName != "qwen-current" {
		t.Fatalf("requiredDefaultLLMModel()=%q want qwen-current", modelName)
	}
}

func TestScrumModelRoutingSourceHasNoHardcodedOrDetachedPath(t *testing.T) {
	source := strings.Join([]string{
		readAPISource(t, "scrum_handlers.go"),
		readAPISource(t, "scrum_coach_config.go"),
		readAPISource(t, "scrum_card_llm_enqueue.go"),
		readAPISource(t, "project_debugger.go"),
	}, "\n")
	for _, forbidden := range []string{
		`"llama3.2"`,
		`"qwen3:4b-thinking"`,
		"context.WithoutCancel",
		"routed.Generation",
		"s.ollamaDefaultModel",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("Scrum model routing contains forbidden path %q", forbidden)
		}
	}
}

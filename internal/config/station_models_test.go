package config

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/station"
)

func TestLoadRejectsBroadProviderDefaultModel(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("WRAPPER_ONLY", "true")
	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv("OLLAMA_MODEL", "base-model")

	if _, err := Load(); err == nil {
		t.Fatal("broad provider model must be rejected")
	}
}

func TestLoadIncludesOnlyExplicitExactStationRoutes(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("WRAPPER_ONLY", "true")
	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv("OMNI_CONVERSATION_RESPONSE_MODEL", "response-model")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.StationModels) != 1 || cfg.StationModels[station.ConversationResponse] != "response-model" {
		t.Fatalf("station routes=%+v", cfg.StationModels)
	}
}

func TestLoadRejectsProviderWithoutExactStationContract(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("WRAPPER_ONLY", "true")
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("EMBEDDING_PROVIDER", "ollama")
	t.Setenv("OLLAMA_EMBEDDING_MODEL", "nomic-test")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "exact prepared station contract") {
		t.Fatalf("Load() error=%v, want exact station provider rejection", err)
	}
}

func TestLoadRejectsUnconsumedGenerationOnlyProviderEnvironment(t *testing.T) {
	for _, key := range []string{"ANTHROPIC_API_KEY", "XAI_BASE_URL", "DEEPSEEK_API_KEY"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "")
			t.Setenv("WRAPPER_ONLY", "true")
			t.Setenv("LLM_PROVIDER", "ollama")
			t.Setenv(key, "configured-but-unconsumed")
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), key+" was removed and is unsupported") {
				t.Fatalf("Load() error=%v, want write-only provider environment rejection", err)
			}
		})
	}
}

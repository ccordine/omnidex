package config

import (
	"os"
	"strings"
	"testing"
)

func TestMain(main *testing.M) {
	if err := os.Setenv("OLLAMA_BASE_URL", "http://localhost:11434"); err != nil {
		panic(err)
	}
	os.Exit(main.Run())
}

func TestMissingOllamaEndpointValidationOccursAtUse(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("WRAPPER_ONLY", "true")
	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv("EMBEDDING_PROVIDER", "ollama")
	t.Setenv("OLLAMA_EMBEDDING_MODEL", "nomic-test")
	t.Setenv("OLLAMA_BASE_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() eagerly validated dormant endpoint: %v", err)
	}
	if err := ValidateProviderConfiguration(cfg, cfg.LLMProvider, "LLM_PROVIDER"); err == nil || !strings.Contains(err.Error(), "OLLAMA_BASE_URL") {
		t.Fatalf("provider use validation error=%v, want explicit OLLAMA_BASE_URL failure", err)
	}
}

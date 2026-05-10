package config

import (
	"strings"
	"testing"
)

func TestLoadOpenAIRequiresAPIKey(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://agent:agent@localhost:5432/agent?sslmode=disable")
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "")

	_, err := Load()
	if err == nil {
		t.Fatalf("expected error when OPENAI_API_KEY is missing")
	}
	if !strings.Contains(err.Error(), "OPENAI_API_KEY") {
		t.Fatalf("expected OPENAI_API_KEY error, got: %v", err)
	}
}

func TestLoadOpenAIUsesOpenAIModelDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://agent:agent@localhost:5432/agent?sslmode=disable")
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_MODEL", "")
	t.Setenv("OPENAI_MODEL_FAST", "")
	t.Setenv("OPENAI_MODEL_REASONING", "")
	t.Setenv("EMBEDDING_MODEL", "legacy-embed")
	t.Setenv("OLLAMA_MODEL", "llama3.2")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.DefaultModel != "gpt-4.1-mini" {
		t.Fatalf("DefaultModel=%q want %q", cfg.DefaultModel, "gpt-4.1-mini")
	}
	if cfg.FastModel != "gpt-4.1-mini" {
		t.Fatalf("FastModel=%q want %q", cfg.FastModel, "gpt-4.1-mini")
	}
	if cfg.ReasoningModel != "gpt-4.1-mini" {
		t.Fatalf("ReasoningModel=%q want %q", cfg.ReasoningModel, "gpt-4.1-mini")
	}
	if cfg.EmbeddingModel != "legacy-embed" {
		t.Fatalf("EmbeddingModel=%q want %q", cfg.EmbeddingModel, "legacy-embed")
	}
}

func TestLoadRejectsUnknownProvider(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://agent:agent@localhost:5432/agent?sslmode=disable")
	t.Setenv("LLM_PROVIDER", "something-else")

	_, err := Load()
	if err == nil {
		t.Fatalf("expected error for unknown provider")
	}
	if !strings.Contains(err.Error(), "LLM_PROVIDER") {
		t.Fatalf("expected LLM_PROVIDER error, got: %v", err)
	}
}

func TestLoadWrapperOnlyAllowsMissingDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("WRAPPER_ONLY", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if !cfg.WrapperOnly {
		t.Fatalf("WrapperOnly=%v want true", cfg.WrapperOnly)
	}
}

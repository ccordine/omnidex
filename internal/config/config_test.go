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

func TestLoadGoogleProviderUsesGoogleModelsAndEmbeddings(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://agent:agent@localhost:5432/agent?sslmode=disable")
	t.Setenv("LLM_PROVIDER", "gemini")
	t.Setenv("GEMINI_API_KEY", "test-key")
	t.Setenv("GEMINI_MODEL", "gemini-test")
	t.Setenv("GEMINI_MODEL_FAST", "gemini-fast")
	t.Setenv("GEMINI_EMBEDDING_MODEL", "text-embedding-test")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.LLMProvider != "google" {
		t.Fatalf("LLMProvider=%q want google", cfg.LLMProvider)
	}
	if cfg.DefaultModel != "gemini-test" {
		t.Fatalf("DefaultModel=%q want gemini-test", cfg.DefaultModel)
	}
	if cfg.FastModel != "gemini-fast" {
		t.Fatalf("FastModel=%q want gemini-fast", cfg.FastModel)
	}
	if cfg.EmbeddingProvider != "google" {
		t.Fatalf("EmbeddingProvider=%q want google", cfg.EmbeddingProvider)
	}
	if cfg.EmbeddingModel != "text-embedding-test" {
		t.Fatalf("EmbeddingModel=%q want text-embedding-test", cfg.EmbeddingModel)
	}
}

func TestLoadAnthropicDefaultsEmbeddingProviderToOllama(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://agent:agent@localhost:5432/agent?sslmode=disable")
	t.Setenv("LLM_PROVIDER", "claude")
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	t.Setenv("ANTHROPIC_MODEL", "claude-test")
	t.Setenv("OLLAMA_EMBEDDING_MODEL", "nomic-test")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.LLMProvider != "anthropic" {
		t.Fatalf("LLMProvider=%q want anthropic", cfg.LLMProvider)
	}
	if cfg.DefaultModel != "claude-test" {
		t.Fatalf("DefaultModel=%q want claude-test", cfg.DefaultModel)
	}
	if cfg.EmbeddingProvider != "ollama" {
		t.Fatalf("EmbeddingProvider=%q want ollama", cfg.EmbeddingProvider)
	}
	if cfg.EmbeddingModel != "nomic-test" {
		t.Fatalf("EmbeddingModel=%q want nomic-test", cfg.EmbeddingModel)
	}
}

func TestLoadHuggingFaceProviderUsesHFTokenAndModel(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://agent:agent@localhost:5432/agent?sslmode=disable")
	t.Setenv("LLM_PROVIDER", "hf")
	t.Setenv("HF_TOKEN", "test-token")
	t.Setenv("HF_MODEL", "org/model:fastest")
	t.Setenv("HF_EMBEDDING_MODEL", "sentence-transformers/test")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.LLMProvider != "huggingface" {
		t.Fatalf("LLMProvider=%q want huggingface", cfg.LLMProvider)
	}
	if cfg.DefaultModel != "org/model:fastest" {
		t.Fatalf("DefaultModel=%q want org/model:fastest", cfg.DefaultModel)
	}
	if cfg.HuggingFaceAPIKey != "test-token" {
		t.Fatalf("HuggingFaceAPIKey not loaded from HF_TOKEN")
	}
	if cfg.EmbeddingModel != "sentence-transformers/test" {
		t.Fatalf("EmbeddingModel=%q want sentence-transformers/test", cfg.EmbeddingModel)
	}
}

func TestLoadXAIProviderUsesGrokAliasesAndOllamaEmbeddings(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://agent:agent@localhost:5432/agent?sslmode=disable")
	t.Setenv("LLM_PROVIDER", "grock")
	t.Setenv("GROK_API_KEY", "xai-test-key")
	t.Setenv("GROK_MODEL", "grok-test")
	t.Setenv("GROK_MODEL_FAST", "grok-fast")
	t.Setenv("OLLAMA_EMBEDDING_MODEL", "nomic-test")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.LLMProvider != "xai" {
		t.Fatalf("LLMProvider=%q want xai", cfg.LLMProvider)
	}
	if cfg.DefaultModel != "grok-test" {
		t.Fatalf("DefaultModel=%q want grok-test", cfg.DefaultModel)
	}
	if cfg.FastModel != "grok-fast" {
		t.Fatalf("FastModel=%q want grok-fast", cfg.FastModel)
	}
	if cfg.XAIAPIKey != "xai-test-key" {
		t.Fatalf("XAIAPIKey not loaded from GROK_API_KEY")
	}
	if cfg.XAIBaseURL != "https://api.x.ai/v1" {
		t.Fatalf("XAIBaseURL=%q want default", cfg.XAIBaseURL)
	}
	if cfg.EmbeddingProvider != "ollama" {
		t.Fatalf("EmbeddingProvider=%q want ollama", cfg.EmbeddingProvider)
	}
	if cfg.EmbeddingModel != "nomic-test" {
		t.Fatalf("EmbeddingModel=%q want nomic-test", cfg.EmbeddingModel)
	}
}

func TestLoadAzureProviderUsesMicrosoftAliases(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://agent:agent@localhost:5432/agent?sslmode=disable")
	t.Setenv("LLM_PROVIDER", "windows-ai")
	t.Setenv("AZURE_OPENAI_ENDPOINT", "https://example.openai.azure.com")
	t.Setenv("AZURE_OPENAI_API_KEY", "azure-test-key")
	t.Setenv("AZURE_OPENAI_DEPLOYMENT", "chat-deployment")
	t.Setenv("AZURE_OPENAI_EMBEDDING_DEPLOYMENT", "embed-deployment")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.LLMProvider != "azure" {
		t.Fatalf("LLMProvider=%q want azure", cfg.LLMProvider)
	}
	if cfg.AzureAIBaseURL != "https://example.openai.azure.com" {
		t.Fatalf("AzureAIBaseURL=%q", cfg.AzureAIBaseURL)
	}
	if cfg.AzureAIAPIKey != "azure-test-key" {
		t.Fatalf("AzureAIAPIKey not loaded from AZURE_OPENAI_API_KEY")
	}
	if cfg.DefaultModel != "chat-deployment" {
		t.Fatalf("DefaultModel=%q want chat-deployment", cfg.DefaultModel)
	}
	if cfg.EmbeddingProvider != "azure" {
		t.Fatalf("EmbeddingProvider=%q want azure", cfg.EmbeddingProvider)
	}
	if cfg.EmbeddingModel != "embed-deployment" {
		t.Fatalf("EmbeddingModel=%q want embed-deployment", cfg.EmbeddingModel)
	}
}

func TestLoadAnthropicCanUseGoogleEmbeddingProvider(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://agent:agent@localhost:5432/agent?sslmode=disable")
	t.Setenv("LLM_PROVIDER", "anthropic")
	t.Setenv("ANTHROPIC_API_KEY", "anthropic-key")
	t.Setenv("EMBEDDING_PROVIDER", "google")
	t.Setenv("GOOGLE_API_KEY", "google-key")
	t.Setenv("GOOGLE_EMBEDDING_MODEL", "text-embedding-004")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.EmbeddingProvider != "google" {
		t.Fatalf("EmbeddingProvider=%q want google", cfg.EmbeddingProvider)
	}
	if cfg.EmbeddingModel != "text-embedding-004" {
		t.Fatalf("EmbeddingModel=%q want text-embedding-004", cfg.EmbeddingModel)
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

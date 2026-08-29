package config

import "testing"

func TestLoadAndValidatePermitNoProviderAuthority(t *testing.T) {
	clearProviderAuthority(t)
	t.Setenv("DATABASE_URL", "")
	t.Setenv("WRAPPER_ONLY", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() without provider authority: %v", err)
	}
	if cfg.LLMProvider != "" || cfg.EmbeddingProvider != "" || cfg.EmbeddingModel != "" {
		t.Fatalf("implicit provider authority remains: generation=%q embedding=%q model=%q",
			cfg.LLMProvider, cfg.EmbeddingProvider, cfg.EmbeddingModel)
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate() resolved absent provider authority at startup: %v", err)
	}
}

func TestLoadDoesNotInferEmbeddingAuthorityFromGenerationAuthority(t *testing.T) {
	clearProviderAuthority(t)
	t.Setenv("DATABASE_URL", "")
	t.Setenv("WRAPPER_ONLY", "true")
	t.Setenv("LLM_PROVIDER", "ollama")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLMProvider != "ollama" || cfg.EmbeddingProvider != "" || cfg.EmbeddingModel != "" {
		t.Fatalf("provider selection=%q/%q model=%q", cfg.LLMProvider, cfg.EmbeddingProvider, cfg.EmbeddingModel)
	}
}

func TestStartupDefersConfiguredProviderValidationUntilUse(t *testing.T) {
	clearProviderAuthority(t)
	t.Setenv("DATABASE_URL", "")
	t.Setenv("WRAPPER_ONLY", "true")
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("EMBEDDING_PROVIDER", "qwen")
	t.Setenv("QWEN_BASE_URL", "not-an-http-url")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() eagerly validated dormant provider authority: %v", err)
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate() eagerly validated dormant provider authority: %v", err)
	}
}

func clearProviderAuthority(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"LLM_PROVIDER", "EMBEDDING_PROVIDER", "EMBEDDING_MODEL",
		"OLLAMA_EMBEDDING_MODEL", "QWEN_EMBEDDING_MODEL",
	} {
		t.Setenv(key, "")
	}
}

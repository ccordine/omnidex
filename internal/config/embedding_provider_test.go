package config

import (
	"strings"
	"testing"
)

func TestLoadChineseOpenAICompatibleEmbeddingProviders(t *testing.T) {
	tests := []struct {
		name, provider, canonical, apiKeyEnv, baseURL string
	}{
		{"qwen", "dashscope", "qwen", "DASHSCOPE_API_KEY", "https://dashscope.aliyuncs.com/compatible-mode/v1"},
		{"zhipu", "glm", "zhipu", "ZHIPU_API_KEY", "https://open.bigmodel.cn/api/paas/v4"},
		{"qianfan", "ernie", "qianfan", "QIANFAN_API_KEY", "https://qianfan.baidubce.com/v2"},
		{"hunyuan", "tencent", "hunyuan", "HUNYUAN_API_KEY", "https://api.hunyuan.cloud.tencent.com/v1"},
		{"baichuan", "baichuan-ai", "baichuan", "BAICHUAN_API_KEY", "https://api.baichuan-ai.com/v1"},
		{"siliconflow", "silicon-flow", "siliconflow", "SILICONFLOW_API_KEY", "https://api.siliconflow.cn/v1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setRemoteProviderTestEnvironment(t)
			t.Setenv("LLM_PROVIDER", "ollama")
			t.Setenv("EMBEDDING_PROVIDER", test.provider)
			t.Setenv("EMBEDDING_MODEL", "provider-embedding")
			t.Setenv(test.apiKeyEnv, "provider-test-key")
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			provider, ok := cfg.CompatibleProviders[test.canonical]
			if cfg.EmbeddingProvider != test.canonical || !ok || provider.APIKey != "provider-test-key" || provider.BaseURL != test.baseURL {
				t.Fatalf("provider config=%+v canonical=%q present=%v", provider, cfg.EmbeddingProvider, ok)
			}
		})
	}
}

func TestLoadRejectsBroadChineseProviderModelID(t *testing.T) {
	setRemoteProviderTestEnvironment(t)
	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv("DEEPSEEK_MODEL", "")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "removed and is unsupported") {
		t.Fatalf("Load() error=%v, want explicit broad model rejection", err)
	}
}

func TestLoadCustomCompatibleProviderRequiresCompleteEndpoint(t *testing.T) {
	setRemoteProviderTestEnvironment(t)
	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv("EMBEDDING_PROVIDER", "openai-compatible")
	t.Setenv("EMBEDDING_MODEL", "compatible-embedding")
	t.Setenv("COMPATIBLE_API_KEY", "test-key")
	t.Setenv("COMPATIBLE_BASE_URL", "")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "COMPATIBLE_BASE_URL") {
		t.Fatalf("Load() error=%v, want COMPATIBLE_BASE_URL failure", err)
	}
}

func TestLoadRejectsMalformedCompatibleProviderURL(t *testing.T) {
	setRemoteProviderTestEnvironment(t)
	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv("EMBEDDING_PROVIDER", "qwen")
	t.Setenv("QWEN_API_KEY", "test-key")
	t.Setenv("QWEN_EMBEDDING_MODEL", "qwen-embedding")
	t.Setenv("QWEN_BASE_URL", "api.qwen.example/v1")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "QWEN_BASE_URL") {
		t.Fatalf("Load() error=%v, want absolute URL failure", err)
	}
}

func TestLoadCompatibleEmbeddingProviderRequiresEmbeddingModel(t *testing.T) {
	setRemoteProviderTestEnvironment(t)
	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv("EMBEDDING_PROVIDER", "qwen")
	t.Setenv("QWEN_API_KEY", "test-key")
	t.Setenv("QWEN_EMBEDDING_MODEL", "")
	t.Setenv("EMBEDDING_MODEL", "")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "QWEN_EMBEDDING_MODEL") {
		t.Fatalf("Load() error=%v, want explicit QWEN_EMBEDDING_MODEL failure", err)
	}
}

func setRemoteProviderTestEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "")
	t.Setenv("WRAPPER_ONLY", "true")
	t.Setenv("EMBEDDING_PROVIDER", "ollama")
	t.Setenv("OLLAMA_EMBEDDING_MODEL", "nomic-test")
}

func TestLoadOpenAIEmbeddingRequiresAPIKey(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://agent:agent@localhost:5432/agent?sslmode=disable")
	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv("EMBEDDING_PROVIDER", "openai")
	t.Setenv("OPENAI_EMBEDDING_MODEL", "text-embedding-test")
	t.Setenv("OPENAI_API_KEY", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err = Validate(cfg); err == nil || !strings.Contains(err.Error(), "OPENAI_API_KEY") {
		t.Fatalf("Validate() error=%v", err)
	}
}

func TestLoadAllowsDatabaseCredentialOverlayBeforeFinalValidation(t *testing.T) {
	setRemoteProviderTestEnvironment(t)
	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv("EMBEDDING_PROVIDER", "qwen")
	t.Setenv("QWEN_EMBEDDING_MODEL", "qwen-embedding")
	t.Setenv("QWEN_API_KEY", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), "QWEN_API_KEY") {
		t.Fatalf("pre-overlay Validate() error=%v", err)
	}
	provider := cfg.CompatibleProviders["qwen"]
	provider.APIKey = "database-qwen-key"
	cfg.CompatibleProviders["qwen"] = provider
	if err := Validate(cfg); err != nil {
		t.Fatalf("post-overlay Validate(): %v", err)
	}
}

func TestLoadEmbeddingProviderModels(t *testing.T) {
	tests := []struct {
		name, provider, key, keyValue, modelKey, model, canonical string
	}{
		{"openai", "openai", "OPENAI_API_KEY", "test-key", "EMBEDDING_MODEL", "legacy-embed", "openai"},
		{"google", "gemini", "GEMINI_API_KEY", "test-key", "GEMINI_EMBEDDING_MODEL", "text-embedding-test", "google"},
		{"huggingface", "hf", "HF_TOKEN", "test-token", "HF_EMBEDDING_MODEL", "sentence-transformers/test", "huggingface"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://agent:agent@localhost:5432/agent?sslmode=disable")
			t.Setenv("LLM_PROVIDER", "ollama")
			t.Setenv("EMBEDDING_PROVIDER", test.provider)
			t.Setenv(test.key, test.keyValue)
			t.Setenv(test.modelKey, test.model)
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.EmbeddingProvider != test.canonical || cfg.EmbeddingModel != test.model {
				t.Fatalf("provider=%q model=%q", cfg.EmbeddingProvider, cfg.EmbeddingModel)
			}
		})
	}
}

func TestLoadAzureEmbeddingProviderUsesMicrosoftAliases(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://agent:agent@localhost:5432/agent?sslmode=disable")
	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv("EMBEDDING_PROVIDER", "windows-ai")
	t.Setenv("AZURE_OPENAI_ENDPOINT", "https://example.openai.azure.com")
	t.Setenv("AZURE_OPENAI_API_KEY", "azure-test-key")
	t.Setenv("AZURE_OPENAI_EMBEDDING_DEPLOYMENT", "embed-deployment")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AzureAIBaseURL != "https://example.openai.azure.com" || cfg.AzureAIAPIKey != "azure-test-key" || cfg.EmbeddingProvider != "azure" || cfg.EmbeddingModel != "embed-deployment" {
		t.Fatalf("azure config=%+v", cfg)
	}
}

func TestLoadExactOllamaStationsCanUseGoogleEmbeddingProvider(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://agent:agent@localhost:5432/agent?sslmode=disable")
	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv("EMBEDDING_PROVIDER", "google")
	t.Setenv("GOOGLE_API_KEY", "google-key")
	t.Setenv("GOOGLE_EMBEDDING_MODEL", "text-embedding-004")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EmbeddingProvider != "google" || cfg.EmbeddingModel != "text-embedding-004" {
		t.Fatalf("provider=%q model=%q", cfg.EmbeddingProvider, cfg.EmbeddingModel)
	}
}

package llmprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/llmprovider/catalog"
)

func TestNewFromConfigRoutesAnthropicGenerationToOllamaEmbeddings(t *testing.T) {
	cfg := config.Config{
		LLMProvider:        "anthropic",
		EmbeddingProvider:  "ollama",
		DefaultModel:       "claude-test",
		EmbeddingModel:     "nomic-test",
		AnthropicAPIKey:    "anthropic-key",
		AnthropicBaseURL:   "https://api.anthropic.com/v1",
		AnthropicVersion:   "2023-06-01",
		AnthropicMaxTokens: 1024,
		OllamaBaseURL:      "http://localhost:11434",
	}

	client, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatalf("NewFromConfig() error: %v", err)
	}
	if _, ok := client.(*llm.RoutedClient); !ok {
		t.Fatalf("client type=%T want *llm.RoutedClient", client)
	}
}

func TestNewProviderSupportsConfiguredRemoteProviders(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		cfg      config.Config
	}{
		{
			name:     "google",
			provider: "google",
			cfg: config.Config{
				GoogleAPIKey:   "google-key",
				GoogleBaseURL:  "https://generativelanguage.googleapis.com/v1beta",
				EmbeddingModel: "text-embedding-004",
			},
		},
		{
			name:     "anthropic",
			provider: "anthropic",
			cfg: config.Config{
				AnthropicAPIKey:    "anthropic-key",
				AnthropicBaseURL:   "https://api.anthropic.com/v1",
				AnthropicVersion:   "2023-06-01",
				AnthropicMaxTokens: 1024,
			},
		},
		{
			name:     "huggingface",
			provider: "huggingface",
			cfg: config.Config{
				HuggingFaceAPIKey:  "hf-token",
				HuggingFaceBaseURL: "https://router.huggingface.co",
				EmbeddingModel:     "sentence-transformers/all-mpnet-base-v2",
			},
		},
		{
			name:     "xai",
			provider: "grok",
			cfg: config.Config{
				CompatibleProviders: map[string]config.CompatibleProviderConfig{
					"xai": {APIKey: "xai-key", BaseURL: "https://api.x.ai/v1"},
				},
			},
		},
		{
			name:     "azure",
			provider: "windows-ai",
			cfg: config.Config{
				AzureAIAPIKey:     "azure-key",
				AzureAIBaseURL:    "https://example.openai.azure.com",
				AzureAIAPIVersion: "2024-10-21",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.cfg.DefaultModel = "test-model"
			client, err := NewProvider(tc.cfg, Options{Provider: tc.provider, Model: "test-model"})
			if err != nil {
				t.Fatalf("NewProvider() error: %v", err)
			}
			if client == nil {
				t.Fatalf("client is nil")
			}
		})
	}
}

func TestNewProviderConstructsEveryOpenAICompatibleCatalogProvider(t *testing.T) {
	for _, definition := range catalog.Definitions() {
		if definition.Protocol != catalog.ProtocolOpenAICompatible {
			continue
		}
		t.Run(definition.ID, func(t *testing.T) {
			cfg := config.Config{
				RequestTimeout: time.Second,
				CompatibleProviders: map[string]config.CompatibleProviderConfig{
					definition.ID: {
						BaseURL: "https://provider.example/v1",
						APIKey:  "provider-key",
					},
				},
			}
			client, err := NewProvider(cfg, Options{Provider: definition.ID, Model: "provider-model"})
			if err != nil {
				t.Fatalf("NewProvider() error: %v", err)
			}
			if client == nil {
				t.Fatal("client is nil")
			}
		})
	}
}

func TestNewProviderChineseCompatibleRequestContract(t *testing.T) {
	var requestPath string
	var authorization string
	var requestedModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		authorization = r.Header.Get("Authorization")
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		requestedModel = body.Model
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"provider response"}}]}`))
	}))
	defer server.Close()

	cfg := config.Config{
		RequestTimeout: time.Second,
		CompatibleProviders: map[string]config.CompatibleProviderConfig{
			"qwen": {
				BaseURL: server.URL + "/compatible-mode/v1",
				APIKey:  "qwen-key",
			},
		},
		ProviderModels: map[string]config.ProviderModelConfig{"qwen": {Default: "qwen-current"}},
	}
	client, err := NewProvider(cfg, Options{Provider: "dashscope"})
	if err != nil {
		t.Fatalf("NewProvider() error: %v", err)
	}
	output, err := client.Generate(context.Background(), "", "contract prompt")
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if output != "provider response" {
		t.Fatalf("Generate()=%q", output)
	}
	if requestPath != "/compatible-mode/v1/chat/completions" {
		t.Fatalf("path=%q", requestPath)
	}
	if authorization != "Bearer qwen-key" {
		t.Fatalf("Authorization=%q", authorization)
	}
	if requestedModel != "qwen-current" {
		t.Fatalf("model=%q", requestedModel)
	}
}

func TestNewProviderRejectsUnknownProviderWithoutOllamaFallback(t *testing.T) {
	client, err := NewProvider(config.Config{OllamaBaseURL: "http://localhost:11434"}, Options{Provider: "not-real", Model: "llama3.2"})
	if err == nil || !strings.Contains(err.Error(), "unsupported LLM provider") {
		t.Fatalf("NewProvider() client=%T error=%v", client, err)
	}
	if client != nil {
		t.Fatalf("unknown provider returned client %T", client)
	}
}

func TestFactorySourceContainsNoUnknownProviderFallback(t *testing.T) {
	source, err := os.ReadFile("factory.go")
	if err != nil {
		t.Fatalf("read factory source: %v", err)
	}
	for _, forbidden := range []string{"func normalizeProvider", `return "ollama"`} {
		if strings.Contains(string(source), forbidden) {
			t.Errorf("factory contains forbidden provider fallback %q", forbidden)
		}
	}
}

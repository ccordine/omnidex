package llmprovider

import (
	"testing"

	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/llm"
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
				XAIAPIKey:  "xai-key",
				XAIBaseURL: "https://api.x.ai/v1",
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

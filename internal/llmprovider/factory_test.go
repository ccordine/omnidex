package llmprovider

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/llmprovider/catalog"
	"github.com/gryph/omnidex/internal/ollama"
	"github.com/gryph/omnidex/internal/openai"
)

func TestNewFromConfigRoutesExactOllamaStationsToHostedEmbeddings(t *testing.T) {
	cfg := config.Config{
		LLMProvider:       "ollama",
		EmbeddingProvider: "qwen",
		EmbeddingModel:    "text-embedding-v4",
		OllamaBaseURL:     "http://localhost:11434",
		RequestTimeout:    time.Second,
		CompatibleProviders: map[string]config.CompatibleProviderConfig{
			"qwen": {BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", APIKey: "qwen-key"},
		},
	}

	transports, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatalf("NewFromConfig() error: %v", err)
	}
	if _, ok := transports.Stations.(*ollama.Client); !ok {
		t.Fatalf("station transport type=%T want *ollama.Client", transports.Stations)
	}
	if _, ok := transports.Embeddings.(*openai.Client); !ok {
		t.Fatalf("embedding transport type=%T want *openai.Client", transports.Embeddings)
	}
}

func TestNewFromConfigRejectsGenericHostedGeneration(t *testing.T) {
	_, err := NewFromConfig(config.Config{LLMProvider: "anthropic", EmbeddingProvider: "ollama"})
	if err == nil || !strings.Contains(err.Error(), "exact prepared station contract") {
		t.Fatalf("NewFromConfig() error=%v, want exact station rejection", err)
	}
}

func TestNewEmbeddingProviderSupportsConfiguredRemoteProviders(t *testing.T) {
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
			name:     "huggingface",
			provider: "huggingface",
			cfg: config.Config{
				HuggingFaceAPIKey:  "hf-token",
				HuggingFaceBaseURL: "https://router.huggingface.co",
				EmbeddingModel:     "sentence-transformers/all-mpnet-base-v2",
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
			definition, ok := catalog.Lookup(tc.provider)
			if !ok {
				t.Fatalf("missing catalog definition for %s", tc.provider)
			}
			client, err := newEmbeddingProvider(tc.cfg, definition, "test-embedding", time.Second)
			if err != nil {
				t.Fatalf("newEmbeddingProvider() error: %v", err)
			}
			if client == nil {
				t.Fatalf("client is nil")
			}
		})
	}
}

func TestNewEmbeddingProviderConstructsEveryOpenAICompatibleProvider(t *testing.T) {
	for _, definition := range catalog.ProductionDefinitions() {
		if definition.Protocol != catalog.ProtocolOpenAICompatible || !definition.SupportsEmbeddings {
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
			client, err := newEmbeddingProvider(cfg, definition, "provider-embedding", time.Second)
			if err != nil {
				t.Fatalf("newEmbeddingProvider() error: %v", err)
			}
			if client == nil {
				t.Fatal("client is nil")
			}
		})
	}
}

func TestNewExactStationProviderRejectsHostedProvider(t *testing.T) {
	definition, ok := catalog.Lookup("qwen")
	if !ok {
		t.Fatal("qwen provider definition is missing")
	}
	client, err := newExactStationProvider(config.Config{}, definition, time.Second)
	if err == nil || !strings.Contains(err.Error(), "exact prepared station contract") {
		t.Fatalf("newExactStationProvider() client=%T error=%v, want exact station rejection", client, err)
	}
}

func TestNewFromConfigRejectsUnknownProviderWithoutOllamaFallback(t *testing.T) {
	transports, err := NewFromConfig(config.Config{
		LLMProvider: "not-real", EmbeddingProvider: "ollama",
	})
	if err == nil || !strings.Contains(err.Error(), "does not implement the exact prepared station contract") {
		t.Fatalf("NewFromConfig() transports=%+v error=%v", transports, err)
	}
}

func TestFactorySourceContainsNoUnknownProviderFallback(t *testing.T) {
	source, err := os.ReadFile("factory.go")
	if err != nil {
		t.Fatalf("read factory source: %v", err)
	}
	for _, forbidden := range []string{
		"func normalizeProvider", `return "ollama"`, "NewRoutedClient", "llm.Client",
	} {
		if strings.Contains(string(source), forbidden) {
			t.Errorf("factory contains forbidden provider fallback %q", forbidden)
		}
	}
}

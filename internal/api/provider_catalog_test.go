package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/llmprovider/catalog"
)

func TestProviderCatalogIsCompleteServerAuthoritativeAndSecretFree(t *testing.T) {
	server := NewServerWithOptions(nil, &fakeLLMClient{}, ServerOptions{
		ProviderConfig: config.Config{
			LLMProvider:       "ollama",
			EmbeddingProvider: "qwen",
			EmbeddingModel:    "text-embedding-v4",
			OllamaBaseURL:     "http://localhost:11434",
			ProviderModels: map[string]config.ProviderModelConfig{
				"qwen": {Embedding: "text-embedding-v4"},
			},
			CompatibleProviders: map[string]config.CompatibleProviderConfig{
				"qwen": {
					BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
					APIKey:  "catalog-secret-must-not-leak",
				},
			},
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/providers", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "catalog-secret-must-not-leak") {
		t.Fatal("provider catalog leaked an API key")
	}
	var payload providerCatalogResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ExactStationProvider != "ollama" || payload.EmbeddingProvider != "qwen" {
		t.Fatalf("selected providers=%q/%q", payload.ExactStationProvider, payload.EmbeddingProvider)
	}
	if len(payload.Providers) != len(catalog.ProductionDefinitions()) {
		t.Fatalf("provider count=%d want %d", len(payload.Providers), len(catalog.ProductionDefinitions()))
	}
	var qwen *providerCatalogItem
	for index := range payload.Providers {
		if payload.Providers[index].ID == "qwen" {
			qwen = &payload.Providers[index]
			break
		}
	}
	if qwen == nil {
		t.Fatal("provider catalog omitted qwen")
	}
	if !qwen.ChineseService || qwen.SelectedForStations || !qwen.SelectedForEmbeddings {
		t.Fatalf("qwen catalog item=%#v", qwen)
	}
	if qwen.SupportsExactStations || qwen.ExactStationsConfigured || !qwen.EmbeddingConfigured {
		t.Fatalf("qwen must be embedding-only in the production catalog: %#v", qwen)
	}
	if qwen.EmbeddingModel != "text-embedding-v4" {
		t.Fatalf("qwen embedding model=%q", qwen.EmbeddingModel)
	}
}

func TestProviderCatalogAdvertisesOnlyExactStationGeneration(t *testing.T) {
	server := NewServerWithOptions(nil, &fakeLLMClient{}, ServerOptions{
		ProviderConfig: config.Config{
			LLMProvider:       "ollama",
			EmbeddingProvider: "ollama",
			EmbeddingModel:    "nomic-test",
			OllamaBaseURL:     "http://localhost:11434",
		},
	})
	payload := server.providerCatalog()
	for _, provider := range payload.Providers {
		if provider.ID == "ollama" {
			if !provider.SupportsExactStations || !provider.ExactStationsConfigured {
				t.Fatalf("Ollama exact station transport=%#v", provider)
			}
			continue
		}
		if provider.SupportsExactStations || provider.ExactStationsConfigured {
			t.Errorf("hosted provider %s advertises unsupported exact inference: %#v", provider.ID, provider)
		}
	}
}

func TestProviderCatalogRejectsMutationMethods(t *testing.T) {
	server := NewServerWithOptions(nil, &fakeLLMClient{}, ServerOptions{})
	req := httptest.NewRequest(http.MethodPost, "/v1/providers", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

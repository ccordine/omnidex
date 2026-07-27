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
			LLMProvider:       "qwen",
			EmbeddingProvider: "qwen",
			DefaultModel:      "qwen-current",
			EmbeddingModel:    "text-embedding-v4",
			ProviderModels: map[string]config.ProviderModelConfig{
				"qwen": {Default: "qwen-current", Embedding: "text-embedding-v4"},
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
	if payload.GenerationProvider != "qwen" || payload.EmbeddingProvider != "qwen" {
		t.Fatalf("selected providers=%q/%q", payload.GenerationProvider, payload.EmbeddingProvider)
	}
	if len(payload.Providers) != len(catalog.Definitions()) {
		t.Fatalf("provider count=%d want %d", len(payload.Providers), len(catalog.Definitions()))
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
	if !qwen.ChineseService || !qwen.SelectedForGeneration || !qwen.SelectedForEmbeddings {
		t.Fatalf("qwen catalog item=%#v", qwen)
	}
	if !qwen.GenerationConfigured || !qwen.EmbeddingConfigured {
		t.Fatalf("qwen should be fully configured: %#v", qwen)
	}
	if qwen.GenerationModel != "qwen-current" || qwen.EmbeddingModel != "text-embedding-v4" {
		t.Fatalf("qwen models=%q/%q", qwen.GenerationModel, qwen.EmbeddingModel)
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

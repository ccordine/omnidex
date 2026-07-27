package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/config"
)

func TestResearchStatusRecognizesEmbeddingModelWithLatestTag(t *testing.T) {
	ollamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]string{
				{"name": "qwen2.5-coder:7b"},
				{"name": "nomic-embed-text:latest"},
			},
		})
	}))
	defer ollamaServer.Close()

	server := NewServerWithOptions(nil, &fakeLLMClient{}, ServerOptions{
		ProviderConfig:   ollamaProviderTestConfig(ollamaServer.URL, "qwen2.5-coder:7b", "nomic-embed-text"),
		WebSearchEnabled: false,
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/status/research", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	var payload researchStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Ollama.EmbeddingAvailable {
		t.Fatalf("embedding should be available when installed as nomic-embed-text:latest: %#v", payload.Ollama)
	}
	if len(payload.Ollama.MissingModels) != 0 {
		t.Fatalf("missing models=%v", payload.Ollama.MissingModels)
	}
}

func TestResearchStatusReportsOllamaReachableAndMissingModels(t *testing.T) {
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]string{{"name": "qwen2.5-coder:7b"}},
		})
	}))
	defer ollama.Close()

	server := NewServerWithOptions(nil, &fakeLLMClient{}, ServerOptions{
		ProviderConfig:   ollamaProviderTestConfig(ollama.URL, "qwen2.5-coder:7b", "nomic-embed-text"),
		WebSearchEnabled: false,
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/status/research", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload researchStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Ollama.Reachable || !payload.GenerationProvider.Reachable {
		t.Fatalf("ollama should be reachable: %#v", payload)
	}
	if payload.ResearchRunnable {
		t.Fatal("research must not be runnable when the configured embedding model is missing")
	}
	if len(payload.Ollama.MissingModels) != 1 || payload.Ollama.MissingModels[0] != "nomic-embed-text" {
		t.Fatalf("missing models=%v", payload.Ollama.MissingModels)
	}
	if payload.Ollama.EmbeddingAvailable {
		t.Fatal("embedding model should be reported unavailable")
	}
}

func TestResearchStatusReportsRemoteProviderAsConfiguredWithoutFakeReachability(t *testing.T) {
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
					APIKey:  "qwen-key",
				},
			},
		},
		WebSearchEnabled: false,
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/status/research", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload researchStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.GenerationProvider.Configured || payload.GenerationProvider.Probed || payload.GenerationProvider.Reachable {
		t.Fatalf("generation status must be configured but explicitly unprobed: %#v", payload.GenerationProvider)
	}
	if payload.GenerationProvider.State != "configured" || payload.GenerationProvider.Model != "qwen-current" {
		t.Fatalf("generation status=%#v", payload.GenerationProvider)
	}
	if !payload.EmbeddingProvider.Configured || payload.EmbeddingProvider.Probed || payload.EmbeddingProvider.Reachable {
		t.Fatalf("embedding status must be configured but explicitly unprobed: %#v", payload.EmbeddingProvider)
	}
	if payload.EmbeddingProvider.Model != "text-embedding-v4" {
		t.Fatalf("embedding status=%#v", payload.EmbeddingProvider)
	}
	if !payload.ResearchRunnable {
		t.Fatalf("configured remote providers should be runnable: %#v", payload)
	}
	if payload.Ollama != nil {
		t.Fatalf("remote-only status must not include an Ollama runtime: %#v", payload.Ollama)
	}
	if !strings.Contains(payload.HTML, "qwen-current") || !strings.Contains(payload.HTML, "configured") {
		t.Fatalf("server-rendered provider status is incomplete: %s", payload.HTML)
	}
	if strings.Contains(payload.HTML, ">Ollama<") {
		t.Fatalf("remote-only status rendered an irrelevant Ollama panel: %s", payload.HTML)
	}
}

func TestResearchStatusReportsOllamaUnreachable(t *testing.T) {
	server := NewServerWithOptions(nil, &fakeLLMClient{}, ServerOptions{
		ProviderConfig:   ollamaProviderTestConfig("http://127.0.0.1:1", "qwen2.5-coder:7b", ""),
		WebSearchEnabled: false,
		WebSearchTimeout: time.Millisecond,
		RequestTimeout:   2 * time.Second,
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/status/research", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload researchStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ResearchRunnable {
		t.Fatal("research should not be runnable when Ollama is unreachable")
	}
	if payload.Ollama.Reachable || payload.GenerationProvider.Reachable {
		t.Fatalf("ollama should be unreachable: %#v", payload)
	}
	if payload.Ollama.LastProviderError == "" {
		t.Fatal("expected clear provider error")
	}
}

func TestNormalizeURLStripsTrailingDotHost(t *testing.T) {
	got := normalizeURL("http://172.20.0.1.:11434")
	want := "http://172.20.0.1:11434"
	if got != want {
		t.Fatalf("normalizeURL()=%q want %q", got, want)
	}
}

func TestRenderResearchStatusHTMLEscapesRuntimeErrors(t *testing.T) {
	markup := renderResearchStatusHTML(researchStatusResponse{
		GenerationProvider: generationProviderStatus{Provider: "qwen", Model: "qwen-current", State: "configured", Configured: true},
		EmbeddingProvider:  generationProviderStatus{Provider: "qwen", Model: "text-embedding-v4", State: "configured", Configured: true},
		WebSearch: webSearchRuntimeStatus{Probes: []webSearchProbeStatus{{
			Provider: "<script>alert(1)</script>",
			Error:    `<img src=x onerror=alert(1)>`,
		}}},
		Warnings: []string{`<svg onload=alert(1)>`},
	})
	for _, unsafe := range []string{"<script>", "<img", "<svg"} {
		if strings.Contains(markup, unsafe) {
			t.Fatalf("server-rendered status contains unescaped markup %q: %s", unsafe, markup)
		}
	}
}

func TestProbeOllamaTagsAllowsSlowTagsResponse(t *testing.T) {
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		time.Sleep(4 * time.Second)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]string{{"name": "qwen2.5-coder:7b"}},
		})
	}))
	defer ollama.Close()

	server := NewServerWithOptions(nil, &fakeLLMClient{}, ServerOptions{
		ProviderConfig:   ollamaProviderTestConfig(ollama.URL, "qwen2.5-coder:7b", ""),
		RequestTimeout:   10 * time.Second,
		WebSearchEnabled: false,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	models, err := server.probeOllamaTags(ctx)
	if err != nil {
		t.Fatalf("probeOllamaTags() error=%v", err)
	}
	if len(models) != 1 || models[0] != "qwen2.5-coder:7b" {
		t.Fatalf("models=%v", models)
	}
}

func ollamaProviderTestConfig(baseURL, generationModel, embeddingModel string) config.Config {
	return config.Config{
		LLMProvider:       "ollama",
		EmbeddingProvider: "ollama",
		OllamaBaseURL:     baseURL,
		DefaultModel:      generationModel,
		EmbeddingModel:    embeddingModel,
		ProviderModels: map[string]config.ProviderModelConfig{
			"ollama": {Default: generationModel, Embedding: embeddingModel},
		},
	}
}

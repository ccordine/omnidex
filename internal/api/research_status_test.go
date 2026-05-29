package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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
		DefaultProvider:      "ollama",
		OllamaBaseURL:        ollamaServer.URL,
		OllamaDefaultModel:   "qwen2.5-coder:7b",
		OllamaEmbeddingModel: "nomic-embed-text",
		WebSearchEnabled:     false,
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
		DefaultProvider:      "ollama",
		OllamaBaseURL:        ollama.URL,
		OllamaDefaultModel:   "qwen2.5-coder:7b",
		OllamaEmbeddingModel: "nomic-embed-text",
		WebSearchEnabled:     false,
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
	if len(payload.Ollama.MissingModels) != 1 || payload.Ollama.MissingModels[0] != "nomic-embed-text" {
		t.Fatalf("missing models=%v", payload.Ollama.MissingModels)
	}
	if payload.Ollama.EmbeddingAvailable {
		t.Fatal("embedding model should be reported unavailable")
	}
}

func TestResearchStatusReportsOllamaUnreachable(t *testing.T) {
	server := NewServerWithOptions(nil, &fakeLLMClient{}, ServerOptions{
		DefaultProvider:    "ollama",
		OllamaBaseURL:      "http://127.0.0.1:1",
		OllamaDefaultModel: "qwen2.5-coder:7b",
		WebSearchEnabled:   false,
		WebSearchTimeout:   time.Millisecond,
		RequestTimeout:     2 * time.Second,
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
		DefaultProvider:    "ollama",
		OllamaBaseURL:      ollama.URL,
		OllamaDefaultModel: "qwen2.5-coder:7b",
		RequestTimeout:     10 * time.Second,
		WebSearchEnabled:   false,
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

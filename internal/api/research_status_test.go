package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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

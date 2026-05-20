package omni

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOllamaClientAppliesStableRuntimeDefaults(t *testing.T) {
	var captured map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"model":"fake","done":true,"message":{"role":"assistant","content":"ok"}}`))
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "fake")
	client.ConfigureRuntime("30s", 2048)

	_, err := client.ChatRaw(context.Background(), OllamaChatRequest{
		Messages: []OllamaMessage{{Role: "user", Content: "hello"}},
		Options:  map[string]interface{}{"temperature": 0},
	})
	if err != nil {
		t.Fatal(err)
	}

	if captured["keep_alive"] != "30s" {
		t.Fatalf("keep_alive = %#v, want 30s", captured["keep_alive"])
	}
	options, ok := captured["options"].(map[string]interface{})
	if !ok {
		t.Fatalf("options = %#v", captured["options"])
	}
	if options["num_ctx"] != float64(2048) {
		t.Fatalf("num_ctx = %#v, want 2048", options["num_ctx"])
	}
	if options["temperature"] != float64(0) {
		t.Fatalf("temperature = %#v, want 0", options["temperature"])
	}
}

func TestOllamaClientRequestOverridesRuntimeDefaults(t *testing.T) {
	var captured map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"model":"fake","done":true,"message":{"role":"assistant","content":"ok"}}`))
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "fake")
	client.ConfigureRuntime("30s", 2048)

	_, err := client.ChatRaw(context.Background(), OllamaChatRequest{
		Messages:  []OllamaMessage{{Role: "user", Content: "hello"}},
		Options:   map[string]interface{}{"num_ctx": 4096},
		KeepAlive: "0",
	})
	if err != nil {
		t.Fatal(err)
	}

	if captured["keep_alive"] != "0" {
		t.Fatalf("keep_alive = %#v, want 0", captured["keep_alive"])
	}
	options := captured["options"].(map[string]interface{})
	if options["num_ctx"] != float64(4096) {
		t.Fatalf("num_ctx = %#v, want 4096", options["num_ctx"])
	}
}

func TestOllamaClientPrewarmReportsLoadProfile(t *testing.T) {
	var captured map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"model":"fake","done":true,"message":{"role":"assistant","content":"ok"},"total_duration":100,"load_duration":25,"prompt_eval_count":2,"eval_count":1}`))
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "fake")
	client.ConfigureRuntime("5m", 1024)

	result, err := client.Prewarm(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Model != "fake" || result.Endpoint != server.URL || result.KeepAlive != "5m" || result.NumCtx != 1024 {
		t.Fatalf("unexpected prewarm result metadata: %#v", result)
	}
	if result.TotalDuration != 100 || result.LoadDuration != 25 || result.PromptEvalCount != 2 || result.EvalCount != 1 {
		t.Fatalf("unexpected prewarm timings: %#v", result)
	}
	if captured["keep_alive"] != "5m" {
		t.Fatalf("keep_alive = %#v, want 5m", captured["keep_alive"])
	}
}

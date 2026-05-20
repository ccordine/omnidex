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

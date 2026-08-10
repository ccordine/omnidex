package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

func TestGeneratePreparedRejectsEmptyMessageContent(t *testing.T) {
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"message":{"role":"assistant","content":""},"done":true}`), nil
	})
	client := New("http://ollama.local", "llama3.2", "nomic-embed-text", 5*time.Second, 32768)
	client.httpClient = &http.Client{Timeout: 5 * time.Second, Transport: transport}
	_, err := client.GeneratePrepared(context.Background(), llm.PreparedModel{
		BaseModel: "qwen3:4b-thinking", ContextModel: "qwen3:4b-thinking",
		Prompt: "Return one typed object.", PromptHint: "Begin.",
		MaxOutputTokens: 512, ContextTokens: 8192, ResponseFormat: llm.ResponseFormatJSON,
	})
	if err == nil || !strings.Contains(err.Error(), "missing message content") {
		t.Fatalf("GeneratePrepared() error=%v, want explicit empty-content failure", err)
	}
}

func TestGeneratePreparedRejectsAmbiguousProviderEnvelope(t *testing.T) {
	t.Parallel()
	for name, response := range map[string]string{
		"duplicate message": `{"message":{"role":"assistant","content":"first"},"message":{"role":"assistant","content":"second"}}`,
		"message alias":     `{"Message":{"role":"assistant","content":"result"}}`,
		"content alias":     `{"message":{"role":"assistant","Content":"result"}}`,
	} {
		response := response
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
				return jsonResponse(http.StatusOK, response), nil
			})
			client := New("http://ollama.local", "llama3.2", "nomic-embed-text", 5*time.Second, 32768)
			client.httpClient = &http.Client{Timeout: 5 * time.Second, Transport: transport}
			_, err := client.GeneratePrepared(context.Background(), llm.PreparedModel{
				BaseModel: "llama3.2", ContextModel: "llama3.2", Prompt: "exact system",
				PromptHint: llm.MinimalGeneratePrompt, MaxOutputTokens: 512, ContextTokens: 8192,
			})
			if err == nil || !strings.Contains(err.Error(), "exact Ollama chat response") {
				t.Fatalf("ambiguous response error=%v", err)
			}
		})
	}
}

func TestGeneratePreparedPreservesExactModelContent(t *testing.T) {
	t.Parallel()
	want := " \n{\"decision\":true}\n "
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK,
			`{"message":{"role":"assistant","content":" \n{\"decision\":true}\n "}}`), nil
	})
	client := New("http://ollama.local", "llama3.2", "nomic-embed-text", 5*time.Second, 32768)
	client.httpClient = &http.Client{Timeout: 5 * time.Second, Transport: transport}
	got, err := client.GeneratePrepared(context.Background(), llm.PreparedModel{
		BaseModel: "llama3.2", ContextModel: "llama3.2", Prompt: "exact system",
		PromptHint: llm.MinimalGeneratePrompt, MaxOutputTokens: 512, ContextTokens: 8192,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("model content=%q want exact %q", got, want)
	}
}

func TestChatEnforcesBoundedJSONForControlPlaneContract(t *testing.T) {
	format, numPredict, temperature := "", 0, -1.0
	thinkValue, thinkPresent := true, false
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		format = asString(payload["format"])
		thinkValue, thinkPresent = payload["think"].(bool)
		options := payload["options"].(map[string]any)
		numPredict = int(options["num_predict"].(float64))
		temperature = options["temperature"].(float64)
		return jsonResponse(http.StatusOK, `{"message":{"role":"assistant","content":"{}"}}`), nil
	})
	client := New("http://ollama.local", "llama3.2", "nomic-embed-text", 5*time.Second, 32768)
	client.httpClient = &http.Client{Timeout: 5 * time.Second, Transport: transport}
	_, err := client.Chat(context.Background(), "qwen2.5-coder:7b",
		"SPECIALIST_INVOCATION:\n{}\n\nCONTROL_PLANE_COMMAND: Return raw JSON.",
		"Begin the control-plane-assigned work now.")
	if err != nil {
		t.Fatal(err)
	}
	if format != "json" || !thinkPresent || thinkValue || numPredict != controlPlaneMaxOutputTokens || temperature != 0 {
		t.Fatalf("format=%q think_present=%t think=%t num_predict=%d temperature=%v",
			format, thinkPresent, thinkValue, numPredict, temperature)
	}
}

func TestGeneratePreparedUsesRoleSpecificOutputBudget(t *testing.T) {
	numPredict, numCtx, format := 0, 0, ""
	thinkValue, thinkPresent := true, false
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		options := payload["options"].(map[string]any)
		numPredict, numCtx = int(options["num_predict"].(float64)), int(options["num_ctx"].(float64))
		format = asString(payload["format"])
		thinkValue, thinkPresent = payload["think"].(bool)
		return jsonResponse(http.StatusOK, `{"message":{"role":"assistant","content":"{}"}}`), nil
	})
	client := New("http://ollama.local", "llama3.2", "nomic-embed-text", 5*time.Second, 32768)
	client.httpClient = &http.Client{Timeout: 5 * time.Second, Transport: transport}
	_, err := client.GeneratePrepared(context.Background(), llm.PreparedModel{
		BaseModel: "qwen2.5-coder:14b", ContextModel: "qwen2.5-coder:14b",
		Prompt: "Return one typed object.", PromptHint: "Begin.", MaxOutputTokens: 512,
		ContextTokens: 16384, ResponseFormat: llm.ResponseFormatJSON,
	})
	if err != nil {
		t.Fatal(err)
	}
	if numPredict != 512 || numCtx != 16384 || format != "json" || !thinkPresent || thinkValue {
		t.Fatalf("predict=%d context=%d format=%q think_present=%t think=%t",
			numPredict, numCtx, format, thinkPresent, thinkValue)
	}
}

func TestGeneratePreparedSendsTheExactJSONSchemaToOllama(t *testing.T) {
	var format map[string]any
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		format, _ = payload["format"].(map[string]any)
		return jsonResponse(http.StatusOK, `{"message":{"role":"assistant","content":"{\"content\":\"package main\\n\"}"}}`), nil
	})
	client := New("http://ollama.local", "qwen2.5-coder:1.5b", "nomic-embed-text", 5*time.Second, 32768)
	client.httpClient = &http.Client{Timeout: 5 * time.Second, Transport: transport}
	_, err := client.GeneratePrepared(context.Background(), llm.PreparedModel{
		BaseModel: "qwen2.5-coder:1.5b", Prompt: "Generate one complete file.", PromptHint: "Begin.",
		MaxOutputTokens: 1024, ContextTokens: 8192, ResponseFormat: llm.ResponseFormatJSON,
		ResponseSchema: map[string]any{
			"type": "object", "properties": map[string]any{"content": map[string]any{"type": "string"}},
			"required": []string{"content"}, "additionalProperties": false,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	properties, _ := format["properties"].(map[string]any)
	if format["type"] != "object" || properties["content"] == nil {
		t.Fatalf("format=%#v", format)
	}
}

func TestGeneratePreparedRejectsExhaustedContextBeforeRequest(t *testing.T) {
	requestCount := 0
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		requestCount++
		return jsonResponse(http.StatusOK, `{"message":{"role":"assistant","content":"{}"}}`), nil
	})
	client := New("http://ollama.local", "llama3.2", "nomic-embed-text", 5*time.Second, 32768)
	client.httpClient = &http.Client{Timeout: 5 * time.Second, Transport: transport}
	_, err := client.GeneratePrepared(context.Background(), llm.PreparedModel{
		BaseModel: "qwen2.5-coder:14b", ContextModel: "qwen2.5-coder:14b",
		Prompt: "CONTROL_PLANE_COMMAND:\n" + strings.Repeat("x", 9000), PromptHint: "Begin.",
		MaxOutputTokens: 512, ContextTokens: 8192,
	})
	if err == nil || !strings.Contains(err.Error(), "inference context exhausted before request") {
		t.Fatalf("GeneratePrepared() error=%v, want explicit context exhaustion", err)
	}
	if requestCount != 0 {
		t.Fatalf("request count=%d, want no truncated model request", requestCount)
	}
}

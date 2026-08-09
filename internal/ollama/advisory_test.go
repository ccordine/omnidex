package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

func TestGeneratePreparedAdvisoryEnablesNativeThinkingAndPreservesBothOutputs(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/api/chat" {
			t.Fatalf("unexpected advisory path %q", request.URL.Path)
		}
		var payload chatRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Stream || payload.Think == nil || !*payload.Think {
			t.Fatalf("advisory request stream=%t think=%v", payload.Stream, payload.Think)
		}
		if payload.Format != nil {
			t.Fatalf("advisory request unexpectedly required format %#v", payload.Format)
		}
		if payload.Options == nil || payload.Options.NumPredict != 1024 || payload.Options.NumCtx != 16384 {
			t.Fatalf("advisory options=%#v", payload.Options)
		}
		return jsonResponse(http.StatusOK, `{"message":{"role":"assistant","thinking":"exact thought","content":"exact memo"},"done":true}`), nil
	})
	client := New("http://ollama.local", "qwen3:4b-thinking", "nomic-embed-text", 5*time.Second, 32768)
	client.httpClient = &http.Client{Timeout: 5 * time.Second, Transport: transport}

	response, err := client.GeneratePreparedAdvisory(context.Background(), llm.PreparedModel{
		BaseModel: "deepseek-r1:8b", ContextModel: "deepseek-r1:8b",
		Prompt: "Analyze one bounded semantic decision.", PromptHint: "Produce the advisory memo.",
		MaxOutputTokens: 1024, ContextTokens: 16384, ThinkingEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Thinking != "exact thought" || response.Content != "exact memo" {
		t.Fatalf("response=%#v", response)
	}
}

func TestGeneratePreparedAdvisoryRejectsSchemaAndDisabledThinking(t *testing.T) {
	client := New("http://ollama.local", "qwen3:4b-thinking", "nomic-embed-text", 5*time.Second, 32768)
	for _, prepared := range []llm.PreparedModel{
		{Prompt: "prompt", PromptHint: "memo", MaxOutputTokens: 128, ContextTokens: 4096},
		{Prompt: "prompt", PromptHint: "memo", MaxOutputTokens: 128, ContextTokens: 4096, ThinkingEnabled: true, ResponseFormat: llm.ResponseFormatJSON},
	} {
		if _, err := client.GeneratePreparedAdvisory(context.Background(), prepared); err == nil {
			t.Fatalf("accepted invalid advisory preparation %#v", prepared)
		}
	}
}

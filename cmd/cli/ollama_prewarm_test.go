package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

func TestProbeOllamaModelUsesBoundedDeterministicRequest(t *testing.T) {
	var request ollamaPrewarmRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/chat":
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"model":"qwen3-coder:30b",
				"message":{"role":"assistant","content":"OK"},
				"done":true,
				"done_reason":"stop",
				"total_duration":2500000000,
				"load_duration":500000000,
				"prompt_eval_count":20,
				"prompt_eval_duration":100000000,
				"eval_count":40,
				"eval_duration":2000000000
			}`))
		case "/api/ps":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"models":[{
				"name":"qwen3-coder:30b",
				"size":22410000384,
				"size_vram":7281586176,
				"context_length":16384
			}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	report, err := probeOllamaModel(context.Background(), server.URL, ollamaPrewarmOptions{
		Model:     "qwen3-coder:30b",
		KeepAlive: "5m",
		NumCtx:    16384,
	})
	if err != nil {
		t.Fatal(err)
	}
	if request.Model != "qwen3-coder:30b" || request.Stream || request.Think {
		t.Fatalf("request routing=%#v", request)
	}
	if request.KeepAlive != "5m" || request.Options.NumCtx != 16384 || request.Options.NumPredict != ollamaPrewarmOutputTokens {
		t.Fatalf("request bounds=%#v", request)
	}
	if request.Options.Temperature != 0 {
		t.Fatalf("temperature=%v want 0", request.Options.Temperature)
	}
	if len(request.Messages) != 1 || request.Messages[0].Role != "user" || request.Messages[0].Content != llm.MinimalGeneratePrompt {
		t.Fatalf("request messages=%#v", request.Messages)
	}
	if report.Model != "qwen3-coder:30b" || report.ContextTokens != 16384 {
		t.Fatalf("report identity=%#v", report)
	}
	if report.TotalDurationMS != 2500 || report.LoadDurationMS != 500 {
		t.Fatalf("report durations=%#v", report)
	}
	if report.PromptTokensPerSecond != 200 || report.EvalTokensPerSecond != 20 {
		t.Fatalf("report throughput=%#v", report)
	}
	if report.AllocatedBytes != 22410000384 || report.VRAMBytes != 7281586176 {
		t.Fatalf("report allocation=%#v", report)
	}
	if report.OffloadPercent < 32.48 || report.OffloadPercent > 32.50 {
		t.Fatalf("offload percent=%f", report.OffloadPercent)
	}
}

func TestProbeOllamaModelFailsLoudly(t *testing.T) {
	tests := []struct {
		name      string
		chat      string
		chatCode  int
		ps        string
		psCode    int
		options   ollamaPrewarmOptions
		wantError string
	}{
		{
			name: "chat failure", chatCode: http.StatusInternalServerError,
			chat: `{"error":"runner crashed"}`, psCode: http.StatusOK, ps: `{"models":[]}`,
			options:   ollamaPrewarmOptions{Model: "qwen", KeepAlive: "5m", NumCtx: 16384},
			wantError: "runner crashed",
		},
		{
			name: "empty output", chatCode: http.StatusOK,
			chat:   `{"model":"qwen","message":{"content":""},"done":true}`,
			psCode: http.StatusOK, ps: `{"models":[]}`,
			options:   ollamaPrewarmOptions{Model: "qwen", KeepAlive: "5m", NumCtx: 16384},
			wantError: "empty content",
		},
		{
			name: "runner absent", chatCode: http.StatusOK,
			chat:   `{"model":"qwen","message":{"content":"OK"},"done":true}`,
			psCode: http.StatusOK, ps: `{"models":[]}`,
			options:   ollamaPrewarmOptions{Model: "qwen", KeepAlive: "5m", NumCtx: 16384},
			wantError: "not present in /api/ps",
		},
		{
			name: "context mismatch", chatCode: http.StatusOK,
			chat:      `{"model":"qwen","message":{"content":"OK"},"done":true}`,
			psCode:    http.StatusOK,
			ps:        `{"models":[{"name":"qwen","size":100,"size_vram":80,"context_length":8192}]}`,
			options:   ollamaPrewarmOptions{Model: "qwen", KeepAlive: "5m", NumCtx: 16384},
			wantError: "context length is 8192, requested 16384",
		},
		{
			name: "invalid context", chatCode: http.StatusOK, chat: `{}`,
			psCode: http.StatusOK, ps: `{}`,
			options:   ollamaPrewarmOptions{Model: "qwen", KeepAlive: "5m", NumCtx: 4096},
			wantError: "inference context tokens",
		},
		{
			name: "invalid keep alive", chatCode: http.StatusOK, chat: `{}`,
			psCode: http.StatusOK, ps: `{}`,
			options:   ollamaPrewarmOptions{Model: "qwen", KeepAlive: "forever", NumCtx: 16384},
			wantError: "keep alive",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/chat" {
					w.WriteHeader(test.chatCode)
					_, _ = w.Write([]byte(test.chat))
					return
				}
				if r.URL.Path == "/api/ps" {
					w.WriteHeader(test.psCode)
					_, _ = w.Write([]byte(test.ps))
					return
				}
				http.NotFound(w, r)
			}))
			t.Cleanup(server.Close)

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err := probeOllamaModel(ctx, server.URL, test.options)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("error=%v want %q", err, test.wantError)
			}
		})
	}
}

func TestOllamaPrewarmDefaultsUseExactFragmentRouteAndContext(t *testing.T) {
	t.Setenv("OLLAMA_MODEL", "qwen3.5:9b-q4_K_M")
	t.Setenv("OLLAMA_MODEL_SPECIALIST_CODING_FRAGMENT", "qwen3-coder:30b")
	t.Setenv("INFERENCE_CONTEXT_TOKENS", "16384")

	if got := ollamaPrewarmDefaultModel(); got != "qwen3-coder:30b" {
		t.Fatalf("default model=%q, want exact fragment route", got)
	}
	got, err := ollamaPrewarmDefaultContext()
	if err != nil {
		t.Fatal(err)
	}
	if got != 16384 {
		t.Fatalf("default context=%d, want 16384", got)
	}
}

func TestOllamaPrewarmDefaultContextRejectsInvalidEnvironment(t *testing.T) {
	t.Setenv("INFERENCE_CONTEXT_TOKENS", "wide")
	_, err := ollamaPrewarmDefaultContext()
	if err == nil || !strings.Contains(err.Error(), "must be an integer") {
		t.Fatalf("error=%v, want explicit integer failure", err)
	}
}

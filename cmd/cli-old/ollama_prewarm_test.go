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

func TestProbeOllamaModelUsesNonInferenceLoadRequest(t *testing.T) {
	var request ollamaPrewarmRequest
	var requestFields map[string]json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/generate":
			var raw json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if err := json.Unmarshal(raw, &request); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(raw, &requestFields); err != nil {
				t.Fatal(err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"model":"qwen3-coder:30b",
				"response":"",
				"done":true,
				"done_reason":"load",
				"total_duration":2500000000,
				"load_duration":500000000,
				"prompt_eval_count":0,
				"eval_count":0
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
	if request.Model != "qwen3-coder:30b" || request.Stream {
		t.Fatalf("request routing=%#v", request)
	}
	if request.KeepAlive != "5m" || request.Options.NumCtx != 16384 {
		t.Fatalf("request bounds=%#v", request)
	}
	for _, forbidden := range []string{"prompt", "messages", "think", "system", "format"} {
		if _, present := requestFields[forbidden]; present {
			t.Fatalf("prewarm request contains inference field %q: %#v", forbidden, requestFields)
		}
	}
	if report.Model != "qwen3-coder:30b" || report.ContextTokens != 16384 {
		t.Fatalf("report identity=%#v", report)
	}
	if report.TotalDurationMS != 2500 || report.LoadDurationMS != 500 {
		t.Fatalf("report durations=%#v", report)
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
		load      string
		loadCode  int
		ps        string
		psCode    int
		options   ollamaPrewarmOptions
		wantError string
	}{
		{
			name: "load failure", loadCode: http.StatusInternalServerError,
			load: `{"error":"runner crashed"}`, psCode: http.StatusOK, ps: `{"models":[]}`,
			options:   ollamaPrewarmOptions{Model: "qwen", KeepAlive: "5m", NumCtx: 16384},
			wantError: "runner crashed",
		},
		{
			name: "unexpected inference", loadCode: http.StatusOK,
			load:   `{"model":"qwen","response":"generated","done":true}`,
			psCode: http.StatusOK, ps: `{"models":[]}`,
			options:   ollamaPrewarmOptions{Model: "qwen", KeepAlive: "5m", NumCtx: 16384},
			wantError: "performed model inference",
		},
		{
			name: "unexpected thinking", loadCode: http.StatusOK,
			load:   `{"model":"qwen","response":"","thinking":"private trace","done":true}`,
			psCode: http.StatusOK, ps: `{"models":[]}`,
			options:   ollamaPrewarmOptions{Model: "qwen", KeepAlive: "5m", NumCtx: 16384},
			wantError: "performed model inference",
		},
		{
			name: "runner absent", loadCode: http.StatusOK,
			load:   `{"model":"qwen","response":"","done":true}`,
			psCode: http.StatusOK, ps: `{"models":[]}`,
			options:   ollamaPrewarmOptions{Model: "qwen", KeepAlive: "5m", NumCtx: 16384},
			wantError: "not present in /api/ps",
		},
		{
			name: "context mismatch", loadCode: http.StatusOK,
			load:      `{"model":"qwen","response":"","done":true}`,
			psCode:    http.StatusOK,
			ps:        `{"models":[{"name":"qwen","size":100,"size_vram":80,"context_length":8192}]}`,
			options:   ollamaPrewarmOptions{Model: "qwen", KeepAlive: "5m", NumCtx: 16384},
			wantError: "context length is 8192, requested 16384",
		},
		{
			name: "invalid context", loadCode: http.StatusOK, load: `{}`,
			psCode: http.StatusOK, ps: `{}`,
			options:   ollamaPrewarmOptions{Model: "qwen", KeepAlive: "5m", NumCtx: 4096},
			wantError: "inference context tokens",
		},
		{
			name: "invalid keep alive", loadCode: http.StatusOK, load: `{}`,
			psCode: http.StatusOK, ps: `{}`,
			options:   ollamaPrewarmOptions{Model: "qwen", KeepAlive: "forever", NumCtx: 16384},
			wantError: "keep alive",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/generate" {
					w.WriteHeader(test.loadCode)
					_, _ = w.Write([]byte(test.load))
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
	t.Setenv("OMNI_CODING_FRAGMENT_MODEL", "qwen3-coder:30b")
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

func TestOllamaPrewarmTimeoutHasThirtyMinuteHardMaximum(t *testing.T) {
	for _, test := range []struct {
		name    string
		timeout time.Duration
		wantErr bool
	}{
		{name: "shorter", timeout: 12 * time.Minute},
		{name: "maximum", timeout: llm.MaximumModelRequestDuration},
		{name: "zero", timeout: 0, wantErr: true},
		{name: "negative", timeout: -time.Second, wantErr: true},
		{name: "over maximum", timeout: llm.MaximumModelRequestDuration + time.Nanosecond, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateOllamaPrewarmTimeout(test.timeout)
			if (err != nil) != test.wantErr {
				t.Fatalf("validate timeout %s error=%v wantErr=%t", test.timeout, err, test.wantErr)
			}
		})
	}
}

func TestOllamaPrewarmPhysicalRequestAddsThirtyMinuteDeadline(t *testing.T) {
	earliest := time.Now().Add(llm.MaximumModelRequestDuration)
	ctx, cancel := ollamaPrewarmRequestContext(context.Background())
	latest := time.Now().Add(llm.MaximumModelRequestDuration)
	deadline, present := ctx.Deadline()
	if !present || deadline.Before(earliest) || deadline.After(latest) {
		cancel()
		t.Fatalf("prewarm request deadline=%s present=%t want within [%s,%s]", deadline, present, earliest, latest)
	}
	cancel()
	select {
	case <-ctx.Done():
	default:
		t.Fatal("prewarm physical request context was not released")
	}
}

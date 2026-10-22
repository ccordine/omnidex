package inferenceapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gryph/omnidex/internal/ollama"
)

const token = "local-inference-test-token-0000000000000000"

func TestInferenceHasNoConversationContinuation(t *testing.T) {
	var calls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Errorf("unexpected provider endpoint: %s", r.URL.Path)
		}
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		for _, forbidden := range []string{"context", "messages", "tools", "format"} {
			if _, ok := body[forbidden]; ok {
				t.Errorf("retained or unrelated input: %s", forbidden)
			}
		}
		if string(body["think"]) != "false" {
			t.Error("inference enabled thinking")
		}
		calls.Add(1)
		fmt.Fprint(w, `{"created_at":"2026-09-20T12:00:00Z","response":"bounded result","done":true,"done_reason":"stop","prompt_eval_count":40,"eval_count":4,"context":[1,2,3]}`)
	}))
	defer provider.Close()
	s, err := New(NewOllamaProvider(ollama.New(provider.URL, "model", ""), "model"), "model", token)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		r := httptest.NewRequest("POST", "/v1/integrations/infer", strings.NewReader(`{"question":"What is the semantic result?","context":"small input","max_output_tokens":4096}`))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"text":"bounded result"`) {
			t.Fatalf("result: %d %s", w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Error("response may be cached")
		}
		if strings.Contains(w.Body.String(), "context") {
			t.Error("provider continuation escaped")
		}
	}
	if calls.Load() != 2 {
		t.Fatal("one request should make exactly one model call")
	}
}

func TestInferenceRejectsInvalidInputBeforeCallingModel(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("invalid input reached provider") }))
	defer provider.Close()
	s, _ := New(NewOllamaProvider(ollama.New(provider.URL, "model", ""), "model"), "model", token)
	for _, tc := range []struct {
		body, auth string
		status     int
	}{
		{`{"question":"q","context":"x","max_output_tokens":4}`, "", 401},
		{`{"question":"q","context":"x","max_output_tokens":4}`, "Bearer wrong", 401},
		{`{"question":"","max_output_tokens":4}`, "Bearer " + token, 400},
		{`{"question":"q","max_output_tokens":4,"tools":[]}`, "Bearer " + token, 400},
		{`{"question":"q","max_output_tokens":4} {}`, "Bearer " + token, 400},
		{strings.Repeat("x", 25000), "Bearer " + token, 400},
	} {
		r := httptest.NewRequest("POST", "/v1/integrations/infer", strings.NewReader(tc.body))
		r.Header.Set("Authorization", tc.auth)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("status %d, expected %d", w.Code, tc.status)
		}
	}
}

func TestInferenceRejectsIncompleteResultsWithoutLeakingContent(t *testing.T) {
	for _, providerBody := range []string{
		`{"created_at":"2026-09-20T12:00:00Z","response":"private utterance","done":false,"prompt_eval_count":5,"eval_count":2}`,
		`{"created_at":"2026-09-20T12:00:00Z","response":"private utterance","done":true,"done_reason":"length","prompt_eval_count":5,"eval_count":2}`,
		`{"error":"private utterance"}`,
	} {
		provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, providerBody) }))
		s, _ := New(NewOllamaProvider(ollama.New(provider.URL, "model", ""), "model"), "model", token)
		r := httptest.NewRequest("POST", "/v1/integrations/infer", bytes.NewBufferString(`{"question":"q","max_output_tokens":10}`))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		provider.Close()
		if w.Code != 502 || strings.Contains(w.Body.String(), "private utterance") {
			t.Fatalf("unsafe result: %d %s", w.Code, w.Body.String())
		}
	}
}

type contextProbeProvider struct {
	predict func(context.Context) (string, error)
}

func (p contextProbeProvider) Predict(ctx context.Context, _ Request) (string, error) {
	return p.predict(ctx)
}
func (p contextProbeProvider) Check(context.Context) error { return nil }

func TestInferenceDoesNotAddADeadlineAndPreservesCancellation(t *testing.T) {
	for _, cancelBefore := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		if cancelBefore {
			cancel()
		}
		called := false
		s, err := New(contextProbeProvider{predict: func(received context.Context) (string, error) {
			called = true
			if _, hasDeadline := received.Deadline(); hasDeadline {
				t.Error("inference added a wall-clock deadline")
			}
			if cancelBefore && received.Err() != context.Canceled {
				t.Error("inference dropped caller cancellation")
			}
			return "result", nil
		}}, "fixture-model", token)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPost, "/v1/integrations/infer", strings.NewReader(`{"question":"q","max_output_tokens":1}`)).WithContext(ctx)
		r.Header.Set("Authorization", "Bearer "+token)
		s.Handler().ServeHTTP(httptest.NewRecorder(), r)
		cancel()
		if !called {
			t.Fatal("provider was not called")
		}
	}
}

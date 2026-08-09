package modelgauntlet

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOllamaGeneratorSendsExactThinkingRequestAndCapturesRunnerEvidence(t *testing.T) {
	var request ollamaChatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/chat":
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode chat request: %v", err)
			}
			_, _ = w.Write([]byte(`{
				"model":"reasoner:8b","message":{"role":"assistant","thinking":"private analysis","content":"advisory memo"},
				"done":true,"total_duration":200,"load_duration":20,"prompt_eval_count":30,
				"prompt_eval_duration":40,"eval_count":50,"eval_duration":60
			}`))
		case "/api/ps":
			_, _ = w.Write([]byte(`{"models":[{"name":"reasoner:8b","size":5200000000,"size_vram":5100000000,"context_length":16384,"digest":"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd","details":{"quantization_level":"Q4_K_M","parameter_size":"8B"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	generator, err := NewOllamaGenerator(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	response, err := generator.Generate(context.Background(), GenerateRequest{
		CaseID: "case", Variant: VariantDeliberated, Stage: StageDeliberation,
		Model: "reasoner:8b", SystemPrompt: "system", UserPrompt: "user", Think: true,
		MaxOutputTokens: 1024, ContextTokens: 16384, KeepAlive: "5m",
	})
	if err != nil {
		t.Fatal(err)
	}
	if request.Model != "reasoner:8b" || request.Think == nil || !*request.Think || request.Stream {
		t.Fatalf("routing request=%#v", request)
	}
	if request.Format != nil || request.KeepAlive != "5m" || request.Options.NumCtx != 16384 || request.Options.NumPredict != 1024 {
		t.Fatalf("bounded request=%#v", request)
	}
	if request.Options.Temperature != 0 || len(request.Messages) != 2 {
		t.Fatalf("deterministic request=%#v", request)
	}
	if response.Thinking != "private analysis" || response.Content != "advisory memo" {
		t.Fatalf("response=%#v", response)
	}
	if response.AllocatedBytes != 5200000000 || response.VRAMBytes != 5100000000 || response.RunnerContextTokens != 16384 {
		t.Fatalf("runner evidence=%#v", response)
	}
	if response.Model != "reasoner:8b" || len(response.ModelDigest) != 64 || response.Quantization != "Q4_K_M" || response.ParameterSize != "8B" {
		t.Fatalf("model identity evidence=%#v", response)
	}
}

func TestOllamaGeneratorSendsSchemaAndDisablesThinkingForStableCall(t *testing.T) {
	var request ollamaChatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/chat":
			_ = json.NewDecoder(r.Body).Decode(&request)
			_, _ = w.Write([]byte(`{"model":"stable:9b","message":{"content":"{\"ok\":true}"},"done":true}`))
		case "/api/ps":
			_, _ = w.Write([]byte(`{"models":[{"name":"stable:9b","size":600,"size_vram":600,"context_length":16384,"digest":"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd","details":{"quantization_level":"Q4_K_M","parameter_size":"9B"}}]}`))
		}
	}))
	t.Cleanup(server.Close)
	generator, _ := NewOllamaGenerator(server.URL, server.Client())
	schema := map[string]any{"type": "object"}
	_, err := generator.Generate(context.Background(), GenerateRequest{
		CaseID: "case", Variant: VariantDirect, Stage: StageDirect, Model: "stable:9b",
		SystemPrompt: "system", UserPrompt: "user", ResponseSchema: schema,
		MaxOutputTokens: 128, ContextTokens: 16384, KeepAlive: "5m",
	})
	if err != nil {
		t.Fatal(err)
	}
	if request.Think == nil || *request.Think || request.Format == nil {
		t.Fatalf("structured request=%#v", request)
	}
}

func TestOllamaGeneratorFailsLoudlyWithoutFallback(t *testing.T) {
	tests := []struct {
		name      string
		chat      string
		ps        string
		wantError string
	}{
		{name: "unfinished", chat: `{"model":"reasoner","message":{"content":"memo"},"done":false}`, ps: `{}`, wantError: "did not complete"},
		{name: "empty reasoning", chat: `{"model":"reasoner","message":{},"done":true}`, ps: `{}`, wantError: "empty thinking and content"},
		{name: "wrong model", chat: `{"model":"other","message":{"content":"memo"},"done":true}`, ps: `{}`, wantError: "returned model"},
		{name: "missing runner", chat: `{"model":"reasoner","message":{"content":"memo"},"done":true}`, ps: `{"models":[]}`, wantError: "not present in /api/ps"},
		{name: "context mismatch", chat: `{"model":"reasoner","message":{"content":"memo"},"done":true}`, ps: `{"models":[{"name":"reasoner","context_length":8192}]}`, wantError: "context length is 8192"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/chat" {
					_, _ = w.Write([]byte(test.chat))
					return
				}
				if r.URL.Path == "/api/ps" {
					_, _ = w.Write([]byte(test.ps))
					return
				}
				http.NotFound(w, r)
			}))
			generator, _ := NewOllamaGenerator(server.URL, server.Client())
			_, err := generator.Generate(context.Background(), GenerateRequest{
				CaseID: "case", Variant: VariantDeliberated, Stage: StageDeliberation, Model: "reasoner",
				SystemPrompt: "system", UserPrompt: "user", Think: true, MaxOutputTokens: 10, ContextTokens: 16384, KeepAlive: "5m",
			})
			server.Close()
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("error=%v want %q", err, test.wantError)
			}
		})
	}
}

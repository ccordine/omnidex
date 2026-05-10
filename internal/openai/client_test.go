package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestGenerateUsesAuthAndReturnsMessage(t *testing.T) {
	var gotAuth string
	var gotOrg string
	var gotProject string
	var gotModel string
	var gotSystem string
	var gotUser string

	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		gotAuth = strings.TrimSpace(r.Header.Get("Authorization"))
		gotOrg = strings.TrimSpace(r.Header.Get("OpenAI-Organization"))
		gotProject = strings.TrimSpace(r.Header.Get("OpenAI-Project"))

		var req chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		gotModel = req.Model
		if len(req.Messages) >= 2 {
			gotSystem = req.Messages[0].Content
			gotUser = req.Messages[1].Content
		}

		return jsonResponse(http.StatusOK, `{"choices":[{"message":{"content":"ok"}}]}`), nil
	})

	client := New("https://api.openai.com/v1", "test-key", "gpt-test", "text-embedding-test", "org-a", "proj-a", time.Second)
	client.httpClient = &http.Client{
		Timeout:   time.Second,
		Transport: transport,
	}
	out, err := client.Generate(context.Background(), "", "system prompt")
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if out != "ok" {
		t.Fatalf("Generate()=%q want %q", out, "ok")
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization=%q want %q", gotAuth, "Bearer test-key")
	}
	if gotOrg != "org-a" {
		t.Fatalf("OpenAI-Organization=%q want %q", gotOrg, "org-a")
	}
	if gotProject != "proj-a" {
		t.Fatalf("OpenAI-Project=%q want %q", gotProject, "proj-a")
	}
	if gotModel != "gpt-test" {
		t.Fatalf("model=%q want %q", gotModel, "gpt-test")
	}
	if strings.TrimSpace(gotSystem) != "system prompt" {
		t.Fatalf("system message=%q want %q", gotSystem, "system prompt")
	}
	if strings.TrimSpace(gotUser) == "" {
		t.Fatalf("expected non-empty user hint")
	}
}

func TestEmbeddingParsesVector(t *testing.T) {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		return jsonResponse(http.StatusOK, `{"data":[{"embedding":[0.1,0.2,0.3]}]}`), nil
	})

	client := New("https://api.openai.com/v1", "test-key", "gpt-test", "text-embedding-test", "", "", time.Second)
	client.httpClient = &http.Client{
		Timeout:   time.Second,
		Transport: transport,
	}
	embed, err := client.Embedding(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Embedding() error: %v", err)
	}
	if len(embed) != 3 {
		t.Fatalf("Embedding() length=%d want 3", len(embed))
	}
}

func TestNormalizeBaseURLAddsSchemeAndVersion(t *testing.T) {
	got := normalizeBaseURL("api.openai.com")
	want := "https://api.openai.com/v1"
	if got != want {
		t.Fatalf("normalizeBaseURL()=%q want %q", got, want)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func jsonResponse(status int, body string) *http.Response {
	resp := &http.Response{
		StatusCode: status,
		Status:     strconv.Itoa(status) + " " + http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	resp.Header.Set("Content-Type", "application/json")
	return resp
}

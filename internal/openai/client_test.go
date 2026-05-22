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

func TestAzureOpenAIUsesDeploymentPathAPIKeyAndVersion(t *testing.T) {
	var gotAPIKey string
	var gotPath string
	var gotQuery string

	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAPIKey = strings.TrimSpace(r.Header.Get("api-key"))
		if strings.TrimSpace(r.Header.Get("Authorization")) != "" {
			t.Fatalf("azure request should not use Authorization header")
		}
		return jsonResponse(http.StatusOK, `{"choices":[{"message":{"content":"azure ok"}}]}`), nil
	})

	client := NewAzureAI("https://example.openai.azure.com", "azure-key", "chat-deployment", "", "2024-10-21", "azure_openai", time.Second)
	client.httpClient = &http.Client{Timeout: time.Second, Transport: transport}

	out, err := client.Generate(context.Background(), "", "system prompt")
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if out != "azure ok" {
		t.Fatalf("Generate()=%q want azure ok", out)
	}
	if gotPath != "/openai/deployments/chat-deployment/chat/completions" {
		t.Fatalf("path=%q want azure deployment chat completions path", gotPath)
	}
	if gotQuery != "api-version=2024-10-21" {
		t.Fatalf("query=%q want api-version=2024-10-21", gotQuery)
	}
	if gotAPIKey != "azure-key" {
		t.Fatalf("api-key=%q want azure-key", gotAPIKey)
	}
}

func TestAzureFoundryUsesModelsPath(t *testing.T) {
	var gotPath string
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotPath = r.URL.Path
		return jsonResponse(http.StatusOK, `{"choices":[{"message":{"content":"foundry ok"}}]}`), nil
	})

	client := NewAzureAI("https://resource.services.ai.azure.com", "azure-key", "gpt-4o", "", "", "foundry", time.Second)
	client.httpClient = &http.Client{Timeout: time.Second, Transport: transport}

	if _, err := client.Generate(context.Background(), "", "system prompt"); err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if gotPath != "/models/chat/completions" {
		t.Fatalf("path=%q want foundry models chat path", gotPath)
	}
}

func TestAzureV1UsesOpenAICompatiblePathAndBearerAuth(t *testing.T) {
	var gotPath string
	var gotAuth string
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotPath = r.URL.Path
		gotAuth = strings.TrimSpace(r.Header.Get("Authorization"))
		if strings.TrimSpace(r.Header.Get("api-key")) != "" {
			t.Fatalf("azure v1 request should not use api-key header")
		}
		return jsonResponse(http.StatusOK, `{"choices":[{"message":{"content":"v1 ok"}}]}`), nil
	})

	client := NewAzureAI("https://example.openai.azure.com", "azure-key", "chat-deployment", "", "", "v1", time.Second)
	client.httpClient = &http.Client{Timeout: time.Second, Transport: transport}

	if _, err := client.Generate(context.Background(), "", "system prompt"); err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if gotPath != "/openai/v1/chat/completions" {
		t.Fatalf("path=%q want v1 chat path", gotPath)
	}
	if gotAuth != "Bearer azure-key" {
		t.Fatalf("Authorization=%q want bearer azure-key", gotAuth)
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

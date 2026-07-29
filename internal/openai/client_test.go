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

	"github.com/gryph/omnidex/internal/llm"
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

	client, err := New("https://api.openai.com/v1", "test-key", "gpt-test", "text-embedding-test", "org-a", "proj-a", time.Second)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
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

func TestGeneratePreparedRequestsJSONObjectForTypedControlPlane(t *testing.T) {
	var responseType string
	var maxTokens int
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var req chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.ResponseFormat != nil {
			responseType = req.ResponseFormat.Type
		}
		maxTokens = req.MaxTokens
		return jsonResponse(http.StatusOK, `{"choices":[{"message":{"content":"{}"}}]}`), nil
	})
	client, err := NewCompatible("deepseek", "DEEPSEEK_API_KEY", "https://api.deepseek.com/v1", "test-key", "deepseek-chat", "", "", "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	client.httpClient = &http.Client{Timeout: time.Second, Transport: transport}
	_, err = client.GeneratePrepared(context.Background(), llm.PreparedModel{
		BaseModel: "deepseek-chat", Prompt: "Return an object", PromptHint: "Begin", MaxOutputTokens: 1024, ResponseFormat: llm.ResponseFormatJSON,
	})
	if err != nil {
		t.Fatal(err)
	}
	if responseType != "json_object" {
		t.Fatalf("response_format.type=%q, want json_object", responseType)
	}
	if maxTokens != 1024 {
		t.Fatalf("max_tokens=%d want 1024", maxTokens)
	}
}

func TestGeneratePreparedRequestsStrictJSONSchemaFromCompatibleProviders(t *testing.T) {
	var responseFormat *chatCompletionResponseFormat
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var req chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		responseFormat = req.ResponseFormat
		return jsonResponse(http.StatusOK, `{"choices":[{"message":{"content":"{\"content\":\"ok\"}"}}]}`), nil
	})
	client, err := NewCompatible("qwen", "QWEN_API_KEY", "https://dashscope.example/v1", "test-key", "qwen-coder", "", "", "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	client.httpClient = &http.Client{Timeout: time.Second, Transport: transport}
	schema := map[string]any{
		"type":                 "object",
		"properties":           map[string]any{"content": map[string]any{"type": "string"}},
		"required":             []string{"content"},
		"additionalProperties": false,
	}
	_, err = client.GeneratePrepared(context.Background(), llm.PreparedModel{
		BaseModel: "qwen-coder", Prompt: "Generate a file", PromptHint: "Begin",
		ResponseFormat: llm.ResponseFormatJSON, ResponseSchema: schema,
	})
	if err != nil {
		t.Fatal(err)
	}
	if responseFormat == nil || responseFormat.Type != "json_schema" || responseFormat.JSONSchema == nil {
		t.Fatalf("response format=%#v", responseFormat)
	}
	if responseFormat.JSONSchema.Strict != true || responseFormat.JSONSchema.Schema["type"] != "object" {
		t.Fatalf("json schema=%#v", responseFormat.JSONSchema)
	}
}

func TestEmbeddingParsesVector(t *testing.T) {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		return jsonResponse(http.StatusOK, `{"data":[{"embedding":[0.1,0.2,0.3]}]}`), nil
	})

	client, err := New("https://api.openai.com/v1", "test-key", "gpt-test", "text-embedding-test", "", "", time.Second)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
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

func TestNewCompatibleRejectsSchemelessBaseURL(t *testing.T) {
	_, err := NewCompatible("custom", "CUSTOM_API_KEY", "api.example.com/v1", "test-key", "test-model", "", "", "", time.Second)
	if err == nil || !strings.Contains(err.Error(), "absolute HTTP(S) URL") {
		t.Fatalf("NewCompatible() error=%v, want absolute URL failure", err)
	}
}

func TestNewCompatibleRejectsMissingEndpointCredentialAndModel(t *testing.T) {
	tests := []struct {
		name      string
		baseURL   string
		apiKey    string
		model     string
		wantError string
	}{
		{name: "endpoint", baseURL: "", apiKey: "key", model: "model", wantError: "base URL is required"},
		{name: "credential", baseURL: "https://api.example.com/v1", apiKey: "", model: "model", wantError: "CUSTOM_API_KEY is required"},
		{name: "model", baseURL: "https://api.example.com/v1", apiKey: "key", model: "", wantError: "default model is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewCompatible("custom", "CUSTOM_API_KEY", test.baseURL, test.apiKey, test.model, "", "", "", time.Second)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("NewCompatible() error=%v want %q", err, test.wantError)
			}
		})
	}
}

func TestGenerateRejectsOversizedProviderResponse(t *testing.T) {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, strings.Repeat("x", maxProviderResponseBytes+1)), nil
	})
	client, err := NewCompatible("qwen", "QWEN_API_KEY", "https://provider.example/v1", "key", "model", "", "", "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	client.httpClient = &http.Client{Timeout: time.Second, Transport: transport}

	_, err = client.Generate(context.Background(), "", "prompt")
	if err == nil || !strings.Contains(err.Error(), "response exceeded") {
		t.Fatalf("Generate() error=%v, want bounded response failure", err)
	}
}

func TestMessageContentRejectsMalformedNonTextParts(t *testing.T) {
	if got := messageContentAsString([]any{map[string]any{"type": "image_url"}}); got != "" {
		t.Fatalf("messageContentAsString()=%q, want empty malformed content", got)
	}
	got := messageContentAsString([]any{
		map[string]any{"type": "image_url", "image_url": "https://example.invalid/image.png"},
		map[string]any{"type": "text", "text": "valid text"},
	})
	if got != "valid text" {
		t.Fatalf("messageContentAsString()=%q want valid text", got)
	}
	if got := messageContentAsString(map[string]any{"text": "not a supported top-level shape"}); got != "" {
		t.Fatalf("messageContentAsString()=%q, want unsupported top-level content rejected", got)
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

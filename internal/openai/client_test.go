package openai

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestEmbeddingUsesAuthAndParsesVector(t *testing.T) {
	var authorization, organization, project string
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/v1/embeddings" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		authorization = request.Header.Get("Authorization")
		organization = request.Header.Get("OpenAI-Organization")
		project = request.Header.Get("OpenAI-Project")
		return jsonResponse(
			http.StatusOK, `{"data":[{"embedding":[0.1,0.2,0.3]}]}`,
		), nil
	})
	client, err := NewEmbedding(
		"https://api.openai.com/v1", "test-key", "text-embedding-test",
		"org-a", "project-a", time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	client.httpClient.Transport = transport
	vector, err := client.Embedding(context.Background(), "hello")
	if err != nil {
		t.Fatal(err)
	}
	if len(vector) != 3 || authorization != "Bearer test-key" ||
		organization != "org-a" || project != "project-a" {
		t.Fatalf(
			"vector=%v authorization=%q organization=%q project=%q",
			vector, authorization, organization, project,
		)
	}
}

func TestAzureEmbeddingUsesDeploymentPathAPIKeyAndVersion(t *testing.T) {
	var path, query, apiKey string
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		path, query = request.URL.Path, request.URL.RawQuery
		apiKey = request.Header.Get("api-key")
		return jsonResponse(http.StatusOK, `{"data":[{"embedding":[1]}]}`), nil
	})
	client, err := NewAzureAIEmbedding(
		"https://example.openai.azure.com", "azure-key", "embedding-deployment",
		"2024-10-21", "azure_openai", time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	client.httpClient.Transport = transport
	if _, err := client.Embedding(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}
	if path != "/openai/deployments/embedding-deployment/embeddings" ||
		query != "api-version=2024-10-21" || apiKey != "azure-key" {
		t.Fatalf("path=%q query=%q api-key=%q", path, query, apiKey)
	}
}

func TestCompatibleEmbeddingRejectsInvalidConfiguration(t *testing.T) {
	for _, test := range []struct {
		name, baseURL, apiKey, model, want string
	}{
		{"endpoint", "", "key", "model", "base URL is required"},
		{"credential", "https://api.example/v1", "", "model", "CUSTOM_API_KEY is required"},
		{"model", "https://api.example/v1", "key", "", "embedding model is required"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewCompatibleEmbedding(
				"custom", "CUSTOM_API_KEY", test.baseURL, test.apiKey, test.model,
				"", "", time.Second,
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v want %q", err, test.want)
			}
		})
	}
}

func TestEmbeddingRejectsOversizedProviderResponse(t *testing.T) {
	client, err := NewCompatibleEmbedding(
		"custom", "CUSTOM_API_KEY", "https://api.example/v1", "key", "model",
		"", "", time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	client.httpClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, strings.Repeat("x", maxProviderResponseBytes+1)), nil
	})
	_, err = client.Embedding(context.Background(), "hello")
	if err == nil || !strings.Contains(err.Error(), "response exceeded") {
		t.Fatalf("error=%v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func jsonResponse(status int, body string) *http.Response {
	response := &http.Response{
		StatusCode: status,
		Status:     strconv.Itoa(status) + " " + http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	response.Header.Set("Content-Type", "application/json")
	return response
}

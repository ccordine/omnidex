package ollama

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

type embeddingRoundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip embeddingRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func TestWrapConnectivityErrorAddsDockerHint(t *testing.T) {
	client := &Client{baseURL: "http://host.docker.internal:11434"}
	baseErr := errors.New(`Post "http://host.docker.internal:11434/api/generate": dial tcp 172.17.0.1:11434: connect: connection refused`)

	got := client.wrapConnectivityError(baseErr, "/api/generate")
	if got == nil {
		t.Fatalf("expected wrapped error")
	}
	if !strings.Contains(got.Error(), "OLLAMA_HOST=0.0.0.0:11434") {
		t.Fatalf("expected docker/ollama hint in error, got: %v", got)
	}
}

func TestWrapConnectivityErrorNoHintForOtherHosts(t *testing.T) {
	client := &Client{baseURL: "http://localhost:11434"}
	baseErr := errors.New(`dial tcp 127.0.0.1:11434: connect: connection refused`)

	got := client.wrapConnectivityError(baseErr, "/api/generate")
	if got == nil {
		t.Fatalf("expected error")
	}
	if got.Error() != baseErr.Error() {
		t.Fatalf("expected unchanged error, got: %v", got)
	}
}

func TestNewUnboundedLeavesResponseDeadlineToCallerContext(t *testing.T) {
	client := NewUnbounded("http://localhost:11434", "model", "", 8192)
	if client.httpClient.Timeout != 0 {
		t.Fatalf("HTTP timeout=%s, want no client response deadline", client.httpClient.Timeout)
	}
}

func TestEmbeddingUsesAuthoritativeEmbedEndpointOnce(t *testing.T) {
	requestCount := 0
	client := &Client{
		baseURL:        "http://ollama.invalid",
		embeddingModel: " nomic-embed-text ",
		httpClient: &http.Client{Transport: embeddingRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			requestCount++
			if request.Method != http.MethodPost {
				t.Errorf("embedding method=%s, want POST", request.Method)
			}
			if request.URL.Path != "/api/embed" {
				t.Errorf("embedding path=%s, want /api/embed", request.URL.Path)
			}
			if request.Header.Get("Content-Type") != "application/json" {
				t.Errorf("embedding content type=%q, want application/json", request.Header.Get("Content-Type"))
			}
			body, err := io.ReadAll(request.Body)
			if err != nil {
				return nil, err
			}
			const want = `{"model":"nomic-embed-text","input":"bounded content"}`
			if string(body) != want {
				t.Errorf("embedding request=%s, want=%s", body, want)
			}
			return embeddingTestResponse(
				request,
				http.StatusOK,
				`{"embeddings":[[0.25,-0.5,0.75]]}`,
			), nil
		})},
	}
	got, err := client.Embedding(context.Background(), "bounded content")
	if err != nil {
		t.Fatalf("Embedding: %v", err)
	}
	if want := []float64{0.25, -0.5, 0.75}; !reflect.DeepEqual(got, want) {
		t.Fatalf("embedding=%v, want=%v", got, want)
	}
	if requestCount != 1 {
		t.Fatalf("provider requests=%d, want exactly one", requestCount)
	}
}

func TestEmbeddingProviderDefectsFailWithoutEndpointFallback(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantError  string
	}{
		{name: "status", statusCode: http.StatusServiceUnavailable, body: "provider offline", wantError: "status=503 body=provider offline"},
		{name: "decode", statusCode: http.StatusOK, body: `{"embeddings":`, wantError: "decode ollama embedding response"},
		{name: "missing vector", statusCode: http.StatusOK, body: `{}`, wantError: "returned 0 vectors"},
		{name: "empty vector", statusCode: http.StatusOK, body: `{"embeddings":[[]]}`, wantError: "returned an empty vector"},
		{name: "multiple vectors", statusCode: http.StatusOK, body: `{"embeddings":[[1],[2]]}`, wantError: "returned 2 vectors"},
		{name: "legacy response shape", statusCode: http.StatusOK, body: `{"embedding":[1]}`, wantError: "returned 0 vectors"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestedPaths := []string{}
			client := &Client{
				baseURL:        "http://ollama.invalid",
				embeddingModel: "nomic-embed-text",
				httpClient: &http.Client{Transport: embeddingRoundTripFunc(func(request *http.Request) (*http.Response, error) {
					requestedPaths = append(requestedPaths, request.URL.Path)
					return embeddingTestResponse(request, test.statusCode, test.body), nil
				})},
			}
			_, err := client.Embedding(context.Background(), "content")
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Embedding error=%v, want containing %q", err, test.wantError)
			}
			if want := []string{"/api/embed"}; !reflect.DeepEqual(requestedPaths, want) {
				t.Fatalf("provider request paths=%v, want=%v", requestedPaths, want)
			}
		})
	}
}

func TestEmbeddingTransportFailureIsNotRetried(t *testing.T) {
	requestCount := 0
	client := &Client{
		baseURL:        "http://ollama.invalid",
		embeddingModel: "nomic-embed-text",
		httpClient: &http.Client{Transport: embeddingRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			requestCount++
			if request.URL.Path != "/api/embed" {
				t.Errorf("embedding path=%s, want /api/embed", request.URL.Path)
			}
			return nil, errors.New("transport exploded")
		})},
	}

	_, err := client.Embedding(context.Background(), "content")
	if err == nil || !strings.Contains(err.Error(), "ollama embedding request") ||
		!strings.Contains(err.Error(), "transport exploded") {
		t.Fatalf("Embedding error=%v, want explicit transport failure", err)
	}
	if requestCount != 1 {
		t.Fatalf("provider requests=%d, want exactly one", requestCount)
	}
}

func embeddingTestResponse(request *http.Request, statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}

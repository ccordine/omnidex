package api

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestMemoryIngestRequiresSuccessfulNonEmptyEmbedding(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{embeddingErr: errors.New("embedding model offline")})
	if _, err := server.requireMemoryEmbedding(context.Background(), "remember this"); err == nil || !strings.Contains(err.Error(), "embedding model offline") {
		t.Fatalf("embedding failure=%v", err)
	}

	server.embeddingClient = &fakeLLMClient{}
	if _, err := server.requireMemoryEmbedding(context.Background(), "remember this"); err == nil || !strings.Contains(err.Error(), "expected 768") {
		t.Fatalf("empty embedding error=%v", err)
	}

	server.embeddingClient = &fakeLLMClient{embedding: make([]float64, 768)}
	vector, err := server.requireMemoryEmbedding(context.Background(), "remember this")
	if err != nil {
		t.Fatal(err)
	}
	if len(vector) != 768 {
		t.Fatalf("embedding length=%d want 768", len(vector))
	}
}

func TestMemoryIngestRejectsInvalidAuthorityBeforeEmbedding(t *testing.T) {
	client := &countingEmbeddingClient{embedding: make([]float64, 768)}
	server := &Server{embeddingClient: client}
	for _, body := range []string{
		`{"scope":{"project_id":0,"channel_id":"channel-one"},"source":"manual","kind":"reference","content":"content","tags":[],"categories":[]}`,
		`{"scope":{"project_id":1,"channel_id":"channel-one"},"source":"","kind":"reference","content":"content","tags":[],"categories":[]}`,
		`{"scope":{"project_id":1,"channel_id":"channel-one"},"source":"manual","kind":"","content":"content","tags":[],"categories":[]}`,
		`{"scope":{"project_id":1,"channel_id":"channel-one"},"source":"manual","kind":"REFERENCE","content":"content","tags":[],"categories":[]}`,
		`{"scope":{"project_id":1,"channel_id":"channel-one"},"source":"manual","kind":"reference","content":"content","tags":[],"categories":["postgres"]}`,
		`{"scope":{"project_id":1,"channel_id":"channel-one"},"source":"manual","kind":"reference","content":"content","tags":["trust:durable"],"categories":[]}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/v1/memory", bytes.NewBufferString(body))
		response := httptest.NewRecorder()
		server.addMemory(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("body=%s status=%d response=%s", body, response.Code, response.Body.String())
		}
	}
	if client.calls != 0 {
		t.Fatalf("invalid memory authority made %d embedding calls", client.calls)
	}
}

func TestMemoryBatchRejectsUnknownFieldsAndInvalidLaterItemBeforeEmbedding(t *testing.T) {
	client := &countingEmbeddingClient{embedding: make([]float64, 768)}
	server := &Server{embeddingClient: client}
	for _, body := range []string{
		`{"memories":[{"scope":{"project_id":1,"channel_id":"channel-one"},"source":"manual","kind":"reference","content":"one","tags":[],"categories":[]}],"fallback":true}`,
		`{"Memories":[{"scope":{"project_id":1,"channel_id":"channel-one"},"source":"manual","kind":"reference","content":"one","tags":[],"categories":[]}]}`,
		`{"memories":[{"scope":{"project_id":1,"channel_id":"channel-one"},"source":"manual","source":"forged","kind":"reference","content":"one","tags":[],"categories":[]}]}`,
		`{"memories":[{"scope":{"project_id":1,"channel_id":"channel-one"},"source":"manual","kind":"reference","content":"one","tags":[],"categories":[]},{"scope":{"project_id":1,"channel_id":"channel-one"},"source":"manual","kind":"unknown","content":"two","tags":[],"categories":[]}]}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/v1/memory/batch", bytes.NewBufferString(body))
		response := httptest.NewRecorder()
		server.addMemoryBatch(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("body=%s status=%d response=%s", body, response.Code, response.Body.String())
		}
	}
	if client.calls != 0 {
		t.Fatalf("invalid memory batch made %d embedding calls", client.calls)
	}
}

type countingEmbeddingClient struct {
	calls     int
	embedding []float64
	failAt    int
}

func (client *countingEmbeddingClient) Embedding(context.Context, string) ([]float64, error) {
	client.calls++
	if client.failAt > 0 && client.calls == client.failAt {
		return nil, errors.New("injected embedding failure")
	}
	return append([]float64(nil), client.embedding...), nil
}

func TestMemoryIngestSourceHasNoEmbeddingFallback(t *testing.T) {
	source, err := os.ReadFile("server_jobs.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"embedding = nil", "works without embeddings", "retrieval will still use tags/fallback"} {
		if strings.Contains(string(source), forbidden) {
			t.Errorf("memory ingest contains forbidden embedding fallback %q", forbidden)
		}
	}
}

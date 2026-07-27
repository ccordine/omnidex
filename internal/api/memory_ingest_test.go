package api

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestMemoryIngestRequiresSuccessfulNonEmptyEmbedding(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{embeddingErr: errors.New("embedding model offline")})
	if _, err := server.requireMemoryEmbedding(context.Background(), "remember this"); err == nil || !strings.Contains(err.Error(), "embedding model offline") {
		t.Fatalf("embedding failure=%v", err)
	}

	server.llmClient = &fakeLLMClient{}
	if _, err := server.requireMemoryEmbedding(context.Background(), "remember this"); err == nil || !strings.Contains(err.Error(), "empty vector") {
		t.Fatalf("empty embedding error=%v", err)
	}

	server.llmClient = &fakeLLMClient{embedding: []float64{0.25, 0.75}}
	vector, err := server.requireMemoryEmbedding(context.Background(), "remember this")
	if err != nil {
		t.Fatal(err)
	}
	if len(vector) != 2 {
		t.Fatalf("embedding length=%d want 2", len(vector))
	}
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

package queue

import (
	"strings"
	"testing"
)

func TestSchemaCreatesSemanticMemoryIndexes(t *testing.T) {
	for _, want := range []string{
		"CREATE EXTENSION IF NOT EXISTS vector",
		"CREATE EXTENSION IF NOT EXISTS pg_trgm",
		"idx_memory_chunks_embedding_hnsw",
		"USING hnsw (embedding vector_cosine_ops)",
		"idx_memory_chunks_content_trgm",
		"USING gin (content gin_trgm_ops)",
	} {
		if !strings.Contains(schemaSQL, want) {
			t.Fatalf("schemaSQL missing %q", want)
		}
	}
}

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

func TestSchemaIncludesScrumBoardOrder(t *testing.T) {
	for _, want := range []string{
		"board_order INT NOT NULL DEFAULT 0",
		"idx_scrum_cards_project_column_order",
	} {
		if !strings.Contains(projectsUISchemaSQL, want) {
			t.Fatalf("projectsUISchemaSQL missing %q", want)
		}
	}
}

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

func TestSchemaIncludesPageLoadIndexes(t *testing.T) {
	for _, want := range []string{
		"idx_jobs_id_desc",
		"idx_jobs_status_id_desc",
		"idx_job_steps_job_sort",
		"idx_job_steps_job_action",
	} {
		if !strings.Contains(schemaSQL, want) {
			t.Fatalf("schemaSQL missing %q", want)
		}
	}
	if !strings.Contains(v3SchemaSQL, "idx_memory_candidates_status") {
		t.Fatal("v3SchemaSQL missing idx_memory_candidates_status")
	}
}

func TestSchemaIncludesScrumBoardOrder(t *testing.T) {
	for _, want := range []string{
		"board_order INT NOT NULL DEFAULT 0",
		"idx_scrum_cards_project_column_order",
		"tags_job_id TEXT NOT NULL DEFAULT ''",
		"ticket_job_id TEXT NOT NULL DEFAULT ''",
	} {
		if !strings.Contains(projectsUISchemaSQL, want) {
			t.Fatalf("projectsUISchemaSQL missing %q", want)
		}
	}
}

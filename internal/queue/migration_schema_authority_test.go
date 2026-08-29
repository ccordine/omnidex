package queue

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegisteredMigrationsOwnBaseSchemaAndOperationalIndexes(t *testing.T) {
	requireMigrationContains(t, "001_init.sql",
		"CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public",
		"CREATE TABLE IF NOT EXISTS jobs", "CREATE TABLE IF NOT EXISTS job_steps",
		"CREATE TABLE IF NOT EXISTS ai_channels", "CREATE TABLE IF NOT EXISTS ai_channel_messages")
	requireMigrationContains(t, "002_v3_artifacts_evidence.sql",
		"CREATE TABLE IF NOT EXISTS memory_candidates")
	requireMigrationContains(t, "003_memory_vector_indexes.sql",
		"CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public",
		"CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public",
		"idx_memory_chunks_embedding_hnsw",
		"idx_memory_chunks_content_trgm")
	requireMigrationContains(t, "004_telemetry_metrics.sql",
		"CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public",
		"CREATE TABLE IF NOT EXISTS omni_runs", "CREATE TABLE IF NOT EXISTS omni_model_calls",
		"idx_omni_events_payload_gin")
	requireMigrationContains(t, "005_projects_ui.sql",
		"CREATE TABLE IF NOT EXISTS ui_sessions", "CREATE TABLE IF NOT EXISTS data_source_channels",
		"CREATE TABLE IF NOT EXISTS data_source_channel_messages")
	requireMigrationContains(t, "012_scrum_board_order.sql",
		"board_order INT NOT NULL DEFAULT 0", "idx_scrum_cards_project_column_order")
	requireMigrationContains(t, "019_page_load_indexes.sql",
		"idx_jobs_id_desc", "idx_jobs_status_id_desc", "idx_job_steps_job_sort")
}

func requireMigrationContains(t *testing.T, name string, fragments ...string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", name))
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range fragments {
		if !strings.Contains(string(raw), fragment) {
			t.Fatalf("migration %s is missing %q", name, fragment)
		}
	}
}

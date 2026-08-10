package queue

import (
	"os"
	"strings"
	"testing"
)

func TestRepositoryIntelligenceMigrationDefinesOneImmutableNormalizedAuthority(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/026_repository_intelligence.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS repository_snapshots",
		"CREATE TABLE IF NOT EXISTS repository_files",
		"CREATE TABLE IF NOT EXISTS repository_exclusions",
		"CREATE TABLE IF NOT EXISTS repository_analyses",
		"CREATE TABLE IF NOT EXISTS repository_analysis_diagnostics",
		"CREATE TABLE IF NOT EXISTS repository_symbols",
		"CREATE TABLE IF NOT EXISTS repository_edges",
		"CREATE TABLE IF NOT EXISTS repository_artifacts",
		"CREATE TABLE IF NOT EXISTS repository_embeddings",
		"gin_trgm_ops",
		"to_tsvector('simple'",
		"prevent_repository_fact_update",
		"repository facts are immutable",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("repository intelligence migration omitted %q", required)
		}
	}
	if strings.Contains(source, "CREATE TABLE IF NOT EXISTS repository_map") {
		t.Fatal("migration created a competing giant repository map")
	}
	for _, factTable := range []string{"repository_symbols", "repository_artifacts", "repository_edges"} {
		marker := "CREATE TABLE IF NOT EXISTS " + factTable
		start := strings.Index(source, marker)
		if start < 0 {
			t.Fatalf("missing %s", factTable)
		}
		end := strings.Index(source[start:], ");")
		if end < 0 || !strings.Contains(source[start:start+end], "analysis_id TEXT NOT NULL") {
			t.Fatalf("%s facts are not bound to one immutable analysis", factTable)
		}
	}
}

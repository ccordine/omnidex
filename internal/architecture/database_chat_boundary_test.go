package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDatabaseChatSemanticStationsCannotReceiveConnectionOrCompiledQueryAuthority(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", ".."))
	paths := []string{
		"internal/assemblyline/database_schema_selection.go",
		"internal/assemblyline/database_query_intent.go",
		"internal/assemblyline/database_evidence_gap.go",
		"internal/assemblyline/database_join_path_selection.go",
		"internal/worker/objective_database_stations.go",
	}
	for _, relative := range paths {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		source := string(raw)
		for _, forbidden := range []string{
			"datasource.Connection", "datasource.CompiledQuery", "DataSourceRecord",
			"BuildPostgresDSN(", "RunSQL(", "ContextPrompt", "Password", "DSN",
		} {
			if strings.Contains(source, forbidden) {
				t.Errorf("%s exposes forbidden model-bound authority %q", relative, forbidden)
			}
		}
	}
}

func TestDatabaseChatWorkflowDoesNotCallRawSQLCompatibilityPath(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", ".."))
	paths, err := filepath.Glob(filepath.Join(root, "internal", "worker", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("worker workflow sources are unavailable")
	}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"RunSQL(", "postgres_query.go", "BuildPostgresDSN(",
			"ui_admin_data_source_results", "data_sources_service",
		} {
			if strings.Contains(string(raw), forbidden) {
				t.Errorf("%s calls forbidden raw-SQL compatibility path %q", filepath.Base(path), forbidden)
			}
		}
	}
}

func TestDatabaseEvidenceReceiptSourceCannotPersistExecutionPayload(t *testing.T) {
	t.Parallel()
	path := filepath.Clean(filepath.Join("..", "queue", "database_evidence_receipts.go"))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"CompiledQuery", "Arguments()", "SQL string", "SQL               string",
		"Password", "DSN", "Connection", "Parameters",
	} {
		if strings.Contains(string(raw), forbidden) {
			t.Errorf("database evidence receipt source exposes forbidden payload %q", forbidden)
		}
	}
}

func TestDataSourceSurfaceHasNoWriteOnlyProfileOrReadOnlyInputControls(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", ".."))
	paths := []string{
		"internal/queue/data_source_types.go",
		"internal/queue/data_sources.go",
		"internal/api/data_sources_service.go",
		"internal/api/ui_admin_data_sources.go",
		"internal/api/web/src/lib/admin_api.ts",
		"internal/api/web/src/controllers/admin_data_sources_controller.ts",
	}
	for _, relative := range paths {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"ContextPrompt", "PrivacyMode", "context_prompt", "privacy_mode", `domain:`, `"domain"`} {
			if strings.Contains(string(raw), forbidden) {
				t.Errorf("%s retains write-only data-source control %q", relative, forbidden)
			}
		}
	}
	for _, relative := range paths[2:] {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{`json:"read_only"`, "read_only: true", "read_only?:"} {
			if strings.Contains(string(raw), forbidden) {
				t.Errorf("%s retains mutable read-only pseudo-control %q", relative, forbidden)
			}
		}
	}
}

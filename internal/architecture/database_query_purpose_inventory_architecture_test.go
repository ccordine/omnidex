package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionHasNoDatabaseQueryCoverageLoops(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", ".."))
	for _, relative := range []string{
		"internal/assemblyline",
		"internal/queue",
		"internal/worker",
	} {
		walkProductionSource(t, filepath.Join(root, relative), func(path, source string) {
			for _, forbidden := range []string{
				"WorkDatabaseQueryProjectionCoverage",
				"WorkDatabaseQueryFilterCoverage",
				"WorkDatabaseQueryFilterValueCoverage",
				"WorkDatabaseQueryWindowCoverage",
				"WorkDatabaseQueryExistenceCoverage",
				"WorkDatabaseQueryHavingCoverage",
				"WorkDatabaseQueryOrderCoverage",
				"DatabaseQueryItemRemains",
				"DatabaseQueryNoUncoveredItem",
				"DatabaseQueryValueRemains",
				"DatabaseQueryNoUncoveredValue",
				"databaseQueryCoveragePrompt",
				"decodeDatabaseQueryCollectionCoverage",
				`"database_query_projection_coverage"`,
				`"database_query_filter_coverage"`,
				`"database_query_filter_value_coverage"`,
				`"database_query_window_coverage"`,
				`"database_query_existence_coverage"`,
				`"database_query_having_coverage"`,
				`"database_query_order_coverage"`,
			} {
				if strings.Contains(source, forbidden) {
					t.Errorf("production source %s retains database query coverage-loop authority %q", path, forbidden)
				}
			}
		})
	}
	schema, err := os.ReadFile(filepath.Join(root, "database", "setup.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"'database_query_projection_coverage'",
		"'database_query_filter_coverage'",
		"'database_query_filter_value_coverage'",
		"'database_query_window_coverage'",
		"'database_query_existence_coverage'",
		"'database_query_having_coverage'",
		"'database_query_order_coverage'",
	} {
		if strings.Contains(string(schema), forbidden) {
			t.Errorf("database schema retains query coverage-loop authority %q", forbidden)
		}
	}
}

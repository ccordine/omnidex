package assemblyline

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDatabaseSchemaSelectionSourceHasNoRetiredCoverageOrGenerationStations(t *testing.T) {
	t.Parallel()
	forbidden := []string{
		"WorkDatabaseSchemaSelectionCoverage",
		"WorkDatabaseSchemaRelationSelection",
		"DatabaseSchemaSelectionLeafInput",
		"NewDatabaseSchemaSelectionCoverageJob",
		"NewDatabaseSchemaRelationSelectionJob",
		"DecodeDatabaseSchemaSelectionCoverageLeaf",
		"DecodeDatabaseSchemaRelationSelectionLeaf",
		`"database_schema_selection_coverage"`,
		`"database_schema_relation_selection"`,
		"'database_schema_selection_coverage'",
		"'database_schema_relation_selection'",
		"RELATION_REMAINS",
		"NO_UNCOVERED_RELATION",
	}
	root := filepath.Clean(filepath.Join("..", ".."))
	files := make([]string, 0)
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, filepath.Join(root, "database", "setup.sql"))
	for _, file := range files {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, retired := range forbidden {
			if strings.Contains(string(source), retired) {
				t.Fatalf("%s retains retired database schema selection source %q", file, retired)
			}
		}
	}
}

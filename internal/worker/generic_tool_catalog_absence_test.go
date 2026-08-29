package worker

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenericModelToolCatalogIsAbsent(t *testing.T) {
	t.Parallel()
	if _, err := os.Stat(filepath.Join("..", "tools")); !os.IsNotExist(err) {
		t.Fatalf("internal/tools must be deleted; stat error=%v", err)
	}

	for _, root := range []string{".", filepath.Join("..", "operation")} {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			source := string(raw)
			for _, forbidden := range []string{
				"internal/tools", "type Registry struct", "type Spec struct",
				"type Schema struct", "type Call struct", "type Handler func",
				"InputSchema", "OutputSchema", "ExecuteOptions", "SpecsFor(",
				"NewRegistry(", "RequireEvidence", "RequireListed",
				"AdditionalProperties", "Aliases []string", "Examples []Example",
			} {
				if strings.Contains(source, forbidden) {
					t.Fatalf("%s retains generic model tool catalog authority %q", path, forbidden)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

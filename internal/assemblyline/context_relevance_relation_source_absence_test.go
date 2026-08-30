package assemblyline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContextRelevanceSelectionControlPlaneIsAbsent(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", ".."))
	paths := []string{
		filepath.Join(root, "internal", "assemblyline"),
		filepath.Join(root, "internal", "contextcompiler"),
		filepath.Join(root, "internal", "worker"),
		filepath.Join(root, "internal", "queue"),
		filepath.Join(root, "database", "setup.sql"),
		filepath.Join(root, "docs", "CHARMELEON_CONTEXT_SYSTEM.md"),
	}
	forbidden := []string{
		"context_relevance_selection",
		"WorkContextRelevanceSelection",
		"ContextRelevanceNoCandidate",
		"ContextRelevanceSelectionInput",
		"AcceptedCandidateIDs",
		"MaxContextRelevanceSelections",
		"validateContextRelevanceReceipt",
	}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		inspect := func(file string) {
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			for _, token := range forbidden {
				if strings.Contains(string(raw), token) {
					t.Errorf("%s retains retired context relevance control %q", file, token)
				}
			}
		}
		if !info.IsDir() {
			inspect(path)
			continue
		}
		err = filepath.WalkDir(path, func(file string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(file) != ".go" ||
				strings.HasSuffix(file, "_test.go") {
				return nil
			}
			inspect(file)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

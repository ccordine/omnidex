package changeapply_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyDeclarationStagingAPIIsAbsent(t *testing.T) {
	t.Parallel()
	assertProductionSourcesOmit(t, ".", map[string]string{
		"func Plan(":                       "legacy staging planner",
		"type Input struct":                "legacy staging input",
		"type CandidateDeclaration struct": "legacy candidate transport",
	})
	assertProductionSourcesOmit(t, filepath.Join("..", "..", "worker"), map[string]string{
		"changeapply.Plan(":                "legacy staging planner caller",
		"changeapply.Input{":               "legacy staging input caller",
		"changeapply.CandidateDeclaration": "legacy candidate transport caller",
	})
}

func assertProductionSourcesOmit(t *testing.T, directory string, forbidden map[string]string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for source, label := range forbidden {
			if strings.Contains(string(raw), source) {
				t.Fatalf("production source %s retains %s %q", entry.Name(), label, source)
			}
		}
	}
}

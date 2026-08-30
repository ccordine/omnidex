package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionHasNoRepositoryEvidenceRelevanceSelectionLoop(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", ".."))
	retiredFile := filepath.Join(
		root,
		"internal",
		"assemblyline",
		"repository_evidence_relevance_leaves.go",
	)
	if _, err := os.Stat(retiredFile); !os.IsNotExist(err) {
		t.Fatalf("retired repository evidence relevance leaf source still exists: %v", err)
	}
	for _, relative := range []string{
		"internal/assemblyline",
		"internal/queue",
		"internal/worker",
	} {
		walkProductionSource(t, filepath.Join(root, relative), func(path, source string) {
			for _, forbidden := range []string{
				"WorkRepositoryEvidenceRelevanceLeaf",
				"RepositoryEvidenceRelevanceLeafInput",
				"RepositoryEvidenceNoRelevantCandidate",
				"NewRepositoryEvidenceRelevanceLeafJob",
				"BuildRepositoryEvidenceRelevanceLeafPrompt",
				"DecodeRepositoryEvidenceRelevanceLeaf",
				`"repository_evidence_relevance_leaf"`,
			} {
				if strings.Contains(source, forbidden) {
					t.Errorf("production source %s retains repository evidence selection-loop authority %q", path, forbidden)
				}
			}
		})
	}
	schema, err := os.ReadFile(filepath.Join(root, "database", "setup.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(schema), "'repository_evidence_relevance_leaf'") {
		t.Error("database schema retains repository evidence selection-loop authority")
	}
}

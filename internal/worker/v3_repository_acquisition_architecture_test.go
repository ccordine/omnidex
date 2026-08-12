package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryAcquisitionHasNoModelSelectedOperationPath(t *testing.T) {
	t.Parallel()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"RepositoryRetrievalDecision",
			"classifyExistingRepositoryRetrieval",
			"RoleCodingRepositoryRetrievalStation",
			"coding_repository_retrieval",
			"WorkRepositoryRetrieval",
			"prepareRepositoryShadowContext",
			"repositoryShadow",
			"ContextProjectionModeShadow",
		} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("production worker %s retains model-selected retrieval authority %q", path, forbidden)
			}
		}
	}
}

func TestRepositorySemanticJobsHaveNoOutputBlindProjectionSidecar(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("v3_coding_workers.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"prepareRepositoryShadowContext",
		"repositoryShadow",
		"ContextProjectionModeShadow",
	} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("portable semantic transport retains output-blind sidecar %q", forbidden)
		}
	}
}

package assemblyline

import (
	"os"
	"strings"
	"testing"
)

func TestAssemblyLineSourceHasNoRejectedRepositoryRetrievalStation(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []string{
		"WorkRepositoryRetrieval",
		"WorkRetrievalBriefing",
		"WorkRetrievalAdvisory",
		"WorkRetrievalSynthesis",
		"RepositoryRetrievalDecision",
		"RepositoryRetrievalBriefingDecision",
		"RepositoryRetrievalAdvisoryInput",
		"RepositoryRetrievalSynthesisInput",
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		source, readErr := os.ReadFile(entry.Name())
		if readErr != nil {
			t.Fatal(readErr)
		}
		for _, rejected := range forbidden {
			if strings.Contains(string(source), rejected) {
				t.Fatalf("%s retains rejected repository retrieval station symbol %q", entry.Name(), rejected)
			}
		}
	}
}

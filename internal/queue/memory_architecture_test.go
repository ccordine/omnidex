package queue

import (
	"os"
	"strings"
	"testing"
)

func TestMemoryCategoryInferenceHasNoContentHeuristics(t *testing.T) {
	if _, err := os.Stat("repository_memory_inference.go"); !os.IsNotExist(err) {
		t.Fatal("mixed memory inference utility file must remain deleted")
	}
	raw, err := os.ReadFile("repository_memory_categories.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, forbidden := range []string{
		"inferMemoryCategories(kind, content string",
		"containsAnyText(",
		"isLanguageMemoryMarker(",
		"isDatabaseMemoryMarker(",
		"isInfrastructureMemoryMarker(",
		"isFrontendMemoryMarker(",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("memory category inference contains forbidden content heuristic %q", forbidden)
		}
	}
}

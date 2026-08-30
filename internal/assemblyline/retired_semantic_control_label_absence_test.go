package assemblyline

import (
	"os"
	"strings"
	"testing"
)

func TestProductionSemanticStationsHaveNoRetiredControlAlternatives(t *testing.T) {
	files := []string{
		"application_intent_leaves.go",
		"roleplay_canon_fact_leaves.go",
		"database_query_intent_leaf_types.go",
		"database_query_intent_leaf_prompt.go",
	}
	retired := []string{
		"CONTEXT_NEEDS_COMPLETE", "COVERAGE_COMPLETE", "SEARCH_ANCHORS_COMPLETE",
		"CONTEXT_TERMS_COMPLETE", "CANON_FACTS_COMPLETE", "SELECTION_COMPLETE",
		"COLLECTION_COMPLETE", "VALUES_COMPLETE", "SEARCH_TERMS_COMPLETE",
		"SEARCH_ANCHOR_REMAINS", "SEARCH_TERM_REMAINS", "CANON_FACT_REMAINS",
		"NO_UNCOVERED_CANON_FACT",
	}
	for _, file := range files {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, label := range retired {
			if strings.Contains(string(source), label) {
				t.Fatalf("%s retains model-authored workflow label %q", file, label)
			}
		}
	}
}

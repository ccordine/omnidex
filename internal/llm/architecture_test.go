package llm

import (
	"os"
	"strings"
	"testing"
)

func TestLLMCoreHasNoHeuristicTagFallback(t *testing.T) {
	raw, err := os.ReadFile("llm.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"SuggestTags(", "SuggestTagsWithModel(", "ParseSuggestedTags(", "fallbackContent"} {
		if strings.Contains(string(raw), forbidden) {
			t.Errorf("LLM core retains removed heuristic tag path %q", forbidden)
		}
	}
}

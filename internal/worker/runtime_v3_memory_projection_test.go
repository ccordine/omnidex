package worker

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/model"
)

func TestV3MemoryProjectionDoesNotInspectObjectivePhrasing(t *testing.T) {
	retrieval := artifacts.RetrievalArtifact{Items: []artifacts.RetrievalItem{{
		ID:      7,
		Kind:    model.MemoryKindReference,
		Content: "A compact historical reference about oranges and apples.",
		Tags:    []string{"project:omnidex", model.MemoryTrustTagApproved},
		Score:   0.91,
	}}}
	first := artifacts.IntentArtifact{UserGoal: "apples", MemoryMode: artifacts.MemoryModeRelevantOnly}
	second := artifacts.IntentArtifact{UserGoal: "完全不同的措辞", MemoryMode: artifacts.MemoryModeRelevantOnly}

	left := projectV3Memory(first, retrieval, "project:omnidex", "", 4)
	right := projectV3Memory(second, retrieval, "project:omnidex", "", 4)
	if !reflect.DeepEqual(left, right) {
		t.Fatalf("projection changed with objective phrasing:\nleft=%+v\nright=%+v", left, right)
	}
	if len(left.References) != 1 || left.References[0].MemoryID != 7 {
		t.Fatalf("semantic retrieval result was not preserved: %+v", left)
	}
}

func TestV3MemorySourceHasNoPhraseOrTokenRelevanceRouter(t *testing.T) {
	for _, path := range []string{"runtime_v3_memory_projection.go", "v3_memory_tools.go"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"significantMemoryTokens",
			"memoryCJKTokens",
			"memoryProjectionStopword",
			"tokenOverlap",
			"no_objective_overlap",
			"rankMemoryOmnibusMatches",
		} {
			if strings.Contains(string(raw), forbidden) {
				t.Errorf("%s contains forbidden memory wording heuristic %q", path, forbidden)
			}
		}
	}
}

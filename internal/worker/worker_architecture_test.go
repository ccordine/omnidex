package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkerHasOneTypedRuntimeWithoutLegacyDecisionEngine(t *testing.T) {
	legacyFiles := []string{
		"engine_analysis_response.go",
		"engine_environment.go",
		"engine_memory_persistence.go",
		"engine_memory_ranking.go",
		"engine_plan_selection.go",
		"engine_planning_tooling.go",
		"engine_prompt_safety.go",
		"engine_response_sources.go",
		"engine_verification.go",
		"engine_verification_decode.go",
		"engine_verification_runtime.go",
		"engine_web_memory.go",
	}
	for _, name := range legacyFiles {
		if _, err := os.Stat(name); err == nil {
			t.Errorf("legacy worker implementation still exists: %s", name)
		} else if !os.IsNotExist(err) {
			t.Fatalf("inspect %s: %v", name, err)
		}
	}

	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []string{
		"webSearchKeywordPattern",
		"complexityKeywordPattern",
		"codeKeywordPattern",
		"explicitHistoricalRecallPattern",
		"explicitWebRequestPattern",
		"hasExplicitApproval(",
		"tokenOverlapScore(",
		"significantTokens(",
		"deriveHeuristicTags(",
		"validateV3IntentGrounding(",
		"withJobModelRouting(",
		"s.models =",
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		for _, needle := range forbidden {
			if strings.Contains(string(raw), needle) {
				t.Errorf("%s contains forbidden heuristic decision path %q", path, needle)
			}
		}
	}
}

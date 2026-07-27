package queue

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOperationsMetricsHaveNoRemovedFallbackEvents(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve operations metrics test path")
	}
	source, err := os.ReadFile(filepath.Join(filepath.Dir(testFile), "operations_metrics.go"))
	if err != nil {
		t.Fatalf("read operations metrics source: %v", err)
	}
	for _, forbidden := range []string{
		"plan_candidate_fallback",
		"web_search_degraded",
		"llm_retry_model",
		"LLM model fallbacks",
	} {
		if strings.Contains(string(source), forbidden) {
			t.Errorf("operations metrics contain removed fallback event %q", forbidden)
		}
	}
}

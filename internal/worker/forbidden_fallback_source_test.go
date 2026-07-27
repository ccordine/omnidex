package worker

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWorkerProductionSourceHasNoFabricatedPlanResponseOrVerificationFallbacks(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve worker test source path")
	}
	sourceDir := filepath.Dir(testFile)
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		t.Fatalf("read worker source directory: %v", err)
	}

	var source strings.Builder
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		content, readErr := os.ReadFile(filepath.Join(sourceDir, entry.Name()))
		if readErr != nil {
			t.Fatalf("read worker production source %s: %v", entry.Name(), readErr)
		}
		source.Write(content)
	}

	for _, forbidden := range []string{
		"fallbackPlanCandidateForInstruction",
		"simpleFileTaskFallbackResponse",
		"fallbackVerificationOutcome",
		"applyVerificationEvaluatorFallbackPolicy",
		"plan_candidate_fallback",
		"verification evaluator fallback",
		"scope_fallback",
		"inferredMemoryScopeTags",
		"llmScopeFallbackModels",
		"shouldRetryWithAlternateModel",
		"hasLocalResearchFallbackContext",
		"web_search_degraded",
		"project_fallback_global",
		"falling back to tag-only",
		"tournament summarize fallback",
		"strategy=simple_file_task tasks=3",
		"externalAgentOptionalForGeneralChat",
		"externalAgentFailureIsFatal",
		"external_agent_skipped",
		"using native research agent",
		"firstNonEmptyString(s.models.Tagging",
		"omni.NewOllamaClient(endpoint",
	} {
		if strings.Contains(source.String(), forbidden) {
			t.Errorf("worker production source contains forbidden fallback path %q", forbidden)
		}
	}
}

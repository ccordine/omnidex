package omni

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestStructuredCoordinatorHasNoCannedApplicationRecoveryFallbacks(t *testing.T) {
	t.Helper()
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	sourceDir := filepath.Dir(testFile)
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		t.Fatalf("read Omni source directory: %v", err)
	}
	var source strings.Builder
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		content, readErr := os.ReadFile(filepath.Join(sourceDir, entry.Name()))
		if readErr != nil {
			t.Fatalf("read Omni production source %s: %v", entry.Name(), readErr)
		}
		source.Write(content)
	}

	for _, forbidden := range []string{
		"fallbackPromptInterpretation",
		"heuristicRoute",
		"sourceVerificationCompletionSatisfied",
		"prompt_interpreter_fallback_objective_added",
		"continuing with deterministic validation",
		"continuing with deterministic checks",
		"deterministicProgressionRecoveryCommand",
		"progression_gate_deterministic_recovery",
		"deterministicZigCLICalculatorRecoveryCommand",
		"deterministicRustOmnidexChessRecoveryCommand",
		"deterministicReactJSONFormatterRecoveryCommand",
		"deterministicReactClockViteRecoveryCommand",
		"deterministicGoReactCalculusRecoveryCommand",
		"ZIG_CALCULATOR_SOURCE_VERIFIED",
		"RUST_OMNIDEX_CHESS_SOURCE_VERIFIED",
	} {
		if strings.Contains(source.String(), forbidden) {
			t.Errorf("structured coordinator contains forbidden canned recovery path %q", forbidden)
		}
	}
}

func TestAppHasNoDeadLocalConversationFallback(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	source, err := os.ReadFile(filepath.Join(filepath.Dir(testFile), "app.go"))
	if err != nil {
		t.Fatalf("read app source: %v", err)
	}
	for _, forbidden := range []string{"conversationReply", "local_fallback", "continuing with local fallback"} {
		if strings.Contains(string(source), forbidden) {
			t.Errorf("app contains forbidden dead conversation fallback %q", forbidden)
		}
	}
}

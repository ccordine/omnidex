package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeV3SourceContainsNoLegacyOrSyntheticCompletionFallback(t *testing.T) {
	paths, err := filepath.Glob("runtime_v3_*.go")
	if err != nil {
		t.Fatal(err)
	}
	subtaskPaths, err := filepath.Glob("v3_subtask_tool_*.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(subtaskPaths) == 0 {
		t.Fatal("no v3 subtask tool implementation files found")
	}
	paths = append(paths, subtaskPaths...)
	forbidden := []string{
		"bestV3FinalFallback",
		"runSubtaskLegacy",
		"answer the user accurately",
		"verification skipped",
		"memorizeSuccessfulJob(",
		"inferMemory(",
		"return \"{}\"",
		"return r.complete(r.action, \"unsupported native v3 action",
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		source := string(raw)
		for _, needle := range forbidden {
			if strings.Contains(source, needle) {
				t.Fatalf("%s retains forbidden fallback %q", path, needle)
			}
		}
	}
}

func TestCodingPostProcessingUsesDeterministicEvidencePath(t *testing.T) {
	for path, required := range map[string]string{
		"runtime_v3_completion.go":   "coding_memory_absent",
		"runtime_v3_synthesis.go":    "buildDeterministicV3CodingAnalysis(",
		"runtime_v3_verification.go": "buildDeterministicV3CodingVerification(",
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), required) {
			t.Errorf("%s does not route completed coding work through %s", path, required)
		}
	}
	synthesis, err := os.ReadFile("runtime_v3_synthesis.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(synthesis), "buildDeterministicV3CodingResponse(") {
		t.Fatal("response drafting does not route completed coding work through deterministic composition")
	}
}

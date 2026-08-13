package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRejectedConversationRuntimeIsAbsent(t *testing.T) {
	t.Parallel()

	removed := []string{
		"runtime_v3_assignment.go",
		"runtime_v3_capabilities.go",
		"runtime_v3_completion.go",
		"runtime_v3_contract.go",
		"runtime_v3_intent_input.go",
		"runtime_v3_invoker.go",
		"runtime_v3_memory_projection.go",
		"runtime_v3_orchestration.go",
		"runtime_v3_plan_validation.go",
		"runtime_v3_repair.go",
		"runtime_v3_research.go",
		"runtime_v3_synthesis.go",
		"runtime_v3_verification.go",
		"v3_subtask_tool_context.go",
		"v3_subtask_tool_execution.go",
		"v3_tool_schema.go",
		"v3_tools.go",
	}
	for _, name := range removed {
		if _, err := os.Stat(name); !os.IsNotExist(err) {
			t.Fatalf("rejected runtime file %s still exists or cannot be checked: %v", name, err)
		}
	}
}

func TestWorkerProductionHasNoRejectedModelOrToolLoop(t *testing.T) {
	t.Parallel()

	forbidden := []string{
		"invokeSpecialist(",
		"executeV3Tool(",
		"newV3ToolRegistry(",
		"tool.registry",
		"v3_subtask_tool_",
	}
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, token := range forbidden {
			if strings.Contains(string(source), token) {
				t.Fatalf("rejected runtime token %q remains in %s", token, path)
			}
		}
	}
}

func TestNativeRuntimeRegistersOnlyCodeOwnedEntryPoints(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("runtime_v3_native.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, required := range []string{`case "objective_resolve":`, `case "v3_coding":`} {
		if !strings.Contains(text, required) {
			t.Fatalf("native runtime omitted %s", required)
		}
	}
	for _, rejected := range []string{
		"v3_intent_parse", "v3_planning", "v3_subtask", "v3_analysis",
		"v3_response_draft", "v3_verification", "v3_memory_review", "v3_finalize",
	} {
		if strings.Contains(text, rejected) {
			t.Fatalf("native runtime still registers rejected action %q", rejected)
		}
	}
}

func TestPortableResponseContractRejectsEveryRemovedBroadScope(t *testing.T) {
	t.Parallel()

	removed := []string{
		"v3_intent_parse", "v3_planning", "v3_subtask_tool_execute",
		"v3_analysis", "v3_response_draft", "v3_verification",
	}
	for _, scope := range removed {
		if _, err := llmResponseContractForScope(scope); err == nil {
			t.Fatalf("removed broad LLM scope %q was accepted", scope)
		}
	}
}

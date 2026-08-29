package worker

import (
	"os"
	"strings"
	"testing"
)

func TestObjectiveSemanticEntrypointsDoNotProjectRawInstruction(t *testing.T) {
	t.Parallel()
	files := []string{
		"objective_turn_workflow.go",
		"objective_turn_runtime.go",
		"objective_conversation_response.go",
		"objective_database_workflow.go",
		"objective_roleplay_research.go",
	}
	forbidden := []string{
		"ExactInstruction: authority.Instruction",
		"ExactRequirement: authority.Instruction",
		"Question:     authority.Instruction",
		"currentNeed := authority.Instruction",
	}
	for _, file := range files {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, pattern := range forbidden {
			if strings.Contains(string(source), pattern) {
				t.Fatalf("%s projects raw instruction through %q", file, pattern)
			}
		}
	}
	runtimeSource, err := os.ReadFile("objective_turn_runtime.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"workspaceScopeForV3Job", "captureExistingRepositoryIndex",
		"captureObjectiveInstructionProvenance", "authority.ModelInstruction",
	} {
		if !strings.Contains(string(runtimeSource), required) {
			t.Fatalf("native objective runtime lacks %q", required)
		}
	}
}

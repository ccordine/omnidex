package codex

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestAgentPlanningAcceptanceUsesFinalizedAuthority(t *testing.T) {
	call := codingagent.Assignment{Role: codingagent.Plan,
		Requirements:      []codingagent.Requirement{{ID: "r1", Outcome: "The module returns the requested result."}},
		ConstructionFacts: []codingagent.ConstructionFact{{Kind: "verification", Quote: "automated tests"}}}
	prompt, err := renderAgentAssignment(call)
	if err != nil {
		t.Fatal(err)
	}
	instruction, assignment, found := strings.Cut(prompt, "\n\nAssignment:\n")
	if !found {
		t.Fatal("planner omitted its exact assignment boundary")
	}
	var decoded codingagent.Assignment
	if err := json.Unmarshal([]byte(assignment), &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(call.Requirements, decoded.Requirements) || !reflect.DeepEqual(call.ConstructionFacts, decoded.ConstructionFacts) || decoded.Objective != nil {
		t.Fatal("task planning substituted authority or received the original objective")
	}
	for _, boundary := range []string{
		"Derive acceptance criteria only from the frozen behavioral requirements and explicit construction facts",
		"Choosing a development method or implementation order does not authorize an additional acceptance obligation",
		"Require historical process evidence only when the supplied construction facts explicitly require that process",
	} {
		if !strings.Contains(instruction, boundary) {
			t.Fatalf("planner lacks authority boundary: %s", boundary)
		}
	}
}

func TestAgentReviewPreservesExplicitAcceptanceWithoutAddingProcess(t *testing.T) {
	for _, acceptance := range [][]string{
		{"The resulting module passes its automated tests."},
		{"The explicitly requested process records the failing test before implementation."},
	} {
		call := codingagent.Assignment{Role: codingagent.Review, Task: &codingagent.Task{ID: "t1", Acceptance: acceptance}}
		prompt, err := renderAgentAssignment(call)
		if err != nil {
			t.Fatal(err)
		}
		instruction, assignment, _ := strings.Cut(prompt, "\n\nAssignment:\n")
		var decoded codingagent.Assignment
		if err := json.Unmarshal([]byte(assignment), &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.Task == nil || !reflect.DeepEqual(decoded.Task.Acceptance, acceptance) || decoded.Objective != nil || len(decoded.Requirements) != 0 {
			t.Fatal("review changed acceptance or gained parent context")
		}
		if !strings.Contains(instruction, "unless the task explicitly requires it") || !strings.Contains(instruction, "Existing tests do not by themselves require historical evidence") {
			t.Fatal("review turned verification into an unrequested historical process")
		}
	}
}

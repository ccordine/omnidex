package codex

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestAgentPlanningAcceptanceUsesFinalizedAuthority(t *testing.T) {
	objective := &codingagent.Objective{Instruction: "Build the requested module."}
	call := codingagent.Assignment{Role: codingagent.Plan,
		Requirements:      []codingagent.Requirement{{ID: "r1", Outcome: "The module returns the requested result."}},
		ConstructionFacts: []codingagent.ConstructionFact{{Kind: "verification", Quote: "automated tests"}}, Objective: objective}
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
	if !reflect.DeepEqual(call.Requirements, decoded.Requirements) || !reflect.DeepEqual(call.ConstructionFacts, decoded.ConstructionFacts) || !reflect.DeepEqual(decoded.Objective, objective) {
		t.Fatal("task planning substituted its exact objective authority")
	}
	for _, boundary := range []string{
		"Derive acceptance criteria only from the objective, its clarifications, and explicit construction facts",
		"Choosing a development method or implementation order does not authorize an additional acceptance obligation",
		"workload-level checks array must also contain one or more executable integration checks",
		"user explicitly approves it",
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
		if !strings.Contains(instruction, "Review result JSON Schema:") || !strings.Contains(instruction, `"verification"`) {
			t.Fatal("native review lacks its exact typed result contract")
		}
	}
}

func TestAgentReviewRequiresIndependentDeclaredRangeChecks(t *testing.T) {
	task := codingagent.Task{ID: "t1", Acceptance: []string{"All declared inputs produce the specified result."}}
	for _, role := range []codingagent.Role{codingagent.Review, codingagent.OverallReview} {
		call := codingagent.Assignment{Role: role, Task: &task}
		if role == codingagent.OverallReview {
			call.Task, call.Workload = nil, &codingagent.Workload{Tasks: []codingagent.Task{task}}
		}
		prompt, err := renderAgentAssignment(call)
		if err != nil {
			t.Fatal(err)
		}
		instruction, assignment, _ := strings.Cut(prompt, "\n\nAssignment:\n")
		var decoded codingagent.Assignment
		if err := json.Unmarshal([]byte(assignment), &decoded); err != nil {
			t.Fatal(err)
		}
		if len(decoded.WriteFiles) != 0 || decoded.Objective != nil {
			t.Fatal("independent checks enlarged the reviewer authority or context")
		}
		if !strings.Contains(instruction, "Execute independent") || !strings.Contains(instruction, "declared input classes and ranges") {
			t.Fatal("review may accept supplied examples without checking declared coverage")
		}
		if role == codingagent.Review {
			for _, boundary := range []string{"representation boundaries, overflow and underflow", "formula operation order", "These checks verify existing criteria and add no requirements"} {
				if !strings.Contains(instruction, boundary) {
					t.Fatalf("task review omitted its existing-behavior check boundary: %s", boundary)
				}
			}
			if decoded.Task == nil || !reflect.DeepEqual(decoded.Task.Acceptance, task.Acceptance) {
				t.Fatal("independent review changed frozen task criteria")
			}
		} else if decoded.Workload == nil || !reflect.DeepEqual(decoded.Workload.Tasks[0].Acceptance, task.Acceptance) {
			t.Fatal("integration review changed frozen task criteria")
		}
	}
}

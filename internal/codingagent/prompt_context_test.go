package codingagent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPlanningSharedContextExcludesCurrentLiteralMessage(t *testing.T) {
	call := Assignment{Role: Plan, Objective: &Objective{Instruction: "Exact current owner message."},
		PlanningConversation: []PlanningMessage{{Role: "user", Text: "Earlier owner message."}, {Role: "assistant", Text: "Earlier plan."}}}
	raw, err := json.Marshal(PromptContext(call))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), call.Objective.Instruction) || !strings.Contains(string(raw), "Earlier owner message.") || !strings.Contains(string(raw), "Earlier plan.") {
		t.Fatalf("shared history lost prior turns or duplicated the current turn: %s", raw)
	}
	if call.Objective == nil || call.Objective.Instruction != "Exact current owner message." {
		t.Fatal("history projection mutated the current request")
	}
}

func TestParserSharedContextExcludesCurrentSourceResponse(t *testing.T) {
	for _, role := range []Role{PlanningInitial, PlanningMutation, ExecutionResponse, ReviewResponse} {
		call := Assignment{Role: role, PlanningResponse: "Exact current planning response.", Response: "Exact current task response."}
		raw, err := json.Marshal(PromptContext(call))
		if err != nil || strings.Contains(string(raw), call.PlanningResponse) || strings.Contains(string(raw), call.Response) {
			t.Fatalf("%s minifies or duplicates the current parser source in shared context: %s %v", role, raw, err)
		}
		if call.PlanningResponse != "Exact current planning response." || call.Response != "Exact current task response." {
			t.Fatal("context projection mutated the exact source response")
		}
	}
}

func TestNativeSharedContextDoesNotDuplicateFrozenInstruction(t *testing.T) {
	for _, role := range []Role{Execute, Review} {
		call := Assignment{Role: role, Task: &Task{Goal: "Exact frozen user instruction.", ReadFiles: []string{"input.txt"}},
			Clarifications: []TaskClarification{{Question: "Which format?", Answer: "CSV"}}}
		raw, err := json.Marshal(PromptContext(call))
		if err != nil || strings.Contains(string(raw), call.Task.Goal) || !strings.Contains(string(raw), "input.txt") || !strings.Contains(string(raw), "CSV") {
			t.Fatalf("%s duplicated the current task or lost its durable context: %s %v", role, raw, err)
		}
	}
}

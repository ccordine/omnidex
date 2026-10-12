package codex

import (
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestAgentReviewSchemaUsesExactAssignedCriteria(t *testing.T) {
	task := codingagent.Task{ID: "task-a", Acceptance: []string{"The value uses the specified rule.", "Accepted inputs return the specified type."}, RequirementIDs: []string{"parent-a"}}
	call := codingagent.Assignment{Role: codingagent.Review, Task: &task, Requirements: []codingagent.Requirement{{ID: "parent-a", Outcome: "Parent outcome."}}}
	item := reviewSchemaItem(t, agentOutputSchema(call))
	properties := item["properties"].(map[string]any)
	if !reflect.DeepEqual(properties["task_id"].(map[string]any)["enum"], []string{task.ID}) ||
		!reflect.DeepEqual(properties["requirement"].(map[string]any)["enum"], task.Acceptance) {
		t.Fatal("review can recombine labels, change task identity, or add parent scope")
	}
	if _, constrained := properties["observation"].(map[string]any)["enum"]; constrained {
		t.Fatal("defect explanation was replaced with a closed structural label")
	}
	if item["additionalProperties"] != false {
		t.Fatal("review finding can add undeclared fields")
	}
}

func TestAgentOverallReviewSchemaPairsEachTaskWithOnlyBoundCriteria(t *testing.T) {
	first := codingagent.Task{ID: "task-a", Acceptance: []string{"A criterion."}, RequirementIDs: []string{"parent-a"}}
	second := codingagent.Task{ID: "task-b", Acceptance: []string{"B criterion."}, RequirementIDs: []string{"parent-b"}}
	call := codingagent.Assignment{Role: codingagent.OverallReview,
		Workload:     &codingagent.Workload{Tasks: []codingagent.Task{first, second}},
		Requirements: []codingagent.Requirement{{ID: "parent-a", Outcome: "A parent outcome."}, {ID: "parent-b", Outcome: "B criterion."}, {ID: "unbound", Outcome: "Unbound outcome."}}}
	variants := reviewSchemaItem(t, agentOutputSchema(call))["anyOf"].([]map[string]any)
	if len(variants) != 2 {
		t.Fatalf("task variants = %d", len(variants))
	}
	for index, want := range [][]string{{"A criterion.", "A parent outcome."}, {"B criterion."}} {
		properties := variants[index]["properties"].(map[string]any)
		if !reflect.DeepEqual(properties["task_id"].(map[string]any)["enum"], []string{call.Workload.Tasks[index].ID}) ||
			!reflect.DeepEqual(properties["requirement"].(map[string]any)["enum"], want) {
			t.Fatal("overall review pairs a task with a sibling or unbound parent criterion")
		}
	}
}

func reviewSchemaItem(t *testing.T, schema map[string]any) map[string]any {
	t.Helper()
	properties := schema["properties"].(map[string]any)
	return properties["findings"].(map[string]any)["items"].(map[string]any)
}

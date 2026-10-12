package codex

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestAgentReviewSchemaUsesExactAssignedCriteria(t *testing.T) {
	task := codingagent.Task{ID: "task-\"a\"", Acceptance: []string{"The value equals \"approved\".", "Accepted inputs return the specified type."}, RequirementIDs: []string{"parent-a"}}
	call := codingagent.Assignment{Role: codingagent.Review, Task: &task, Requirements: []codingagent.Requirement{{ID: "parent-a", Outcome: "Parent outcome."}}}
	item := reviewSchemaItem(t, agentOutputSchema(call))
	properties := item["properties"].(map[string]any)
	if !reflect.DeepEqual(properties["task_id"].(map[string]any)["enum"], []string{"task_1"}) ||
		!reflect.DeepEqual(properties["requirement"].(map[string]any)["enum"], []string{"criterion_1", "criterion_2"}) {
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
	for index, want := range [][]string{{"criterion_1", "parent_1"}, {"criterion_1"}} {
		properties := variants[index]["properties"].(map[string]any)
		if !reflect.DeepEqual(properties["task_id"].(map[string]any)["enum"], []string{fmt.Sprintf("task_%d", index+1)}) ||
			!reflect.DeepEqual(properties["requirement"].(map[string]any)["enum"], want) {
			t.Fatal("overall review pairs a task with a sibling or unbound parent criterion")
		}
	}
}

func TestAgentReviewSchemaContainsNoQuotedNativeLiterals(t *testing.T) {
	task := codingagent.Task{ID: "task\"identity", Acceptance: []string{"Return \"exact\" without changing it."}, RequirementIDs: []string{"parent\"identity"}}
	for _, role := range []codingagent.Role{codingagent.Review, codingagent.OverallReview} {
		call := codingagent.Assignment{Role: role, Task: &task}
		if role == codingagent.OverallReview {
			call.Task = nil
			call.Workload = &codingagent.Workload{Tasks: []codingagent.Task{task}}
			call.Requirements = []codingagent.Requirement{{ID: "parent\"identity", Outcome: "Preserve \"quoted\" outcomes."}}
		}
		item := reviewSchemaItem(t, agentOutputSchema(call))
		variants := []map[string]any{item}
		if role == codingagent.OverallReview {
			variants = item["anyOf"].([]map[string]any)
		}
		for _, variant := range variants {
			properties := variant["properties"].(map[string]any)
			for _, property := range []string{"task_id", "requirement"} {
				for _, literal := range properties[property].(map[string]any)["enum"].([]string) {
					if strings.Contains(literal, `"`) {
						t.Errorf("%s schema embeds a quoted native string literal: %q", role, literal)
					}
				}
			}
		}
	}
}

func reviewSchemaItem(t *testing.T, schema map[string]any) map[string]any {
	t.Helper()
	properties := schema["properties"].(map[string]any)
	return properties["findings"].(map[string]any)["items"].(map[string]any)
}

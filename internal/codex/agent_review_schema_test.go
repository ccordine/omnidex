package codex

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestAgentReviewSchemaUsesOnlyExactTaskCriteria(t *testing.T) {
	task := codingagent.Task{ID: "task-\"a\"", Acceptance: []string{"The value equals \"approved\".", "Accepted inputs return the specified type."}}
	schema := agentOutputSchema(codingagent.Assignment{Role: codingagent.ReviewResponse, Task: &task})
	if !reflect.DeepEqual(schema["required"], []string{"kind", "findings", "verification"}) {
		t.Fatalf("review schema leaves declared output fields optional: %#v", schema["required"])
	}
	item := reviewSchemaItem(t, schema)
	properties := item["properties"].(map[string]any)
	if properties["task_id"] != nil || !reflect.DeepEqual(properties["requirement"].(map[string]any)["enum"], []string{"criterion_1", "criterion_2"}) {
		t.Fatal("review schema changed or expanded the task packet")
	}
	for _, property := range []string{"requirement"} {
		for _, literal := range properties[property].(map[string]any)["enum"].([]string) {
			if strings.Contains(literal, `"`) {
				t.Fatalf("review schema embeds packet text instead of opaque references: %q", literal)
			}
		}
	}
	if item["additionalProperties"] != false {
		t.Fatal("review finding can add undeclared fields")
	}
}

func TestAgentExecutionSchemaHasNoModelSelectedResultSlots(t *testing.T) {
	for _, role := range []codingagent.Role{codingagent.Plan, codingagent.Execute, codingagent.Review} {
		if schema := agentOutputSchema(codingagent.Assignment{Role: role}); schema != nil {
			t.Fatalf("%s retained a structured Codex response schema: %#v", role, schema)
		}
	}
}

func reviewSchemaItem(t *testing.T, schema map[string]any) map[string]any {
	t.Helper()
	properties := schema["properties"].(map[string]any)
	return properties["findings"].(map[string]any)["items"].(map[string]any)
}

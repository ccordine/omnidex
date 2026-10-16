package codex

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestAgentReviewSchemaUsesOnlyExactTaskCriteria(t *testing.T) {
	task := codingagent.Task{ID: "task-\"a\"", Acceptance: []string{"The value equals \"approved\".", "Accepted inputs return the specified type."}}
	item := reviewSchemaItem(t, agentOutputSchema(codingagent.Assignment{Role: codingagent.Review, Task: &task}))
	properties := item["properties"].(map[string]any)
	if !reflect.DeepEqual(properties["task_id"].(map[string]any)["enum"], []string{"task_1"}) ||
		!reflect.DeepEqual(properties["requirement"].(map[string]any)["enum"], []string{"criterion_1", "criterion_2"}) {
		t.Fatal("review schema changed or expanded the task packet")
	}
	for _, property := range []string{"task_id", "requirement"} {
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

func reviewSchemaItem(t *testing.T, schema map[string]any) map[string]any {
	t.Helper()
	properties := schema["properties"].(map[string]any)
	return properties["findings"].(map[string]any)["items"].(map[string]any)
}

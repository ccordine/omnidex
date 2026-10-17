package codex

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestExecutionAndReviewReceiveOnlyExactTaskPacket(t *testing.T) {
	task := codingagent.Task{ID: "task", Goal: "Implement it",
		Guidance:             []codingagent.TaskGuidance{{ID: "constraint", Kind: "constraint", Text: "Do not change the public contract."}},
		KnownAffectedSystems: []string{"request path"}, Acceptance: []string{"It works."},
		ReadFiles: []string{"input.go"}, WriteFiles: []string{"output.go"},
		Dependencies: []string{"internal-scheduler-edge"}, RequirementIDs: []string{"internal-requirement"},
		AcceptanceCriterionIDs: []string{"internal-criterion"}, Checks: []codingagent.Check{{ID: "test", Argv: []string{"go", "test"}}},
	}
	for _, role := range []codingagent.Role{codingagent.Execute, codingagent.Review} {
		call := codingagent.Assignment{Role: role, Task: &task,
			ReadFiles: []string{"input.go", "output.go"}, Route: codingagent.Route{Provider: "codex", Model: "model"}}
		if role == codingagent.Execute {
			call.WriteFiles = []string{"output.go"}
		}
		prompt, err := renderAgentAssignment(call)
		if err != nil {
			t.Fatal(err)
		}
		instruction := prompt
		context, err := renderAgentContext(call)
		if err != nil {
			t.Fatal(err)
		}
		_, assignment, found := strings.Cut(context, "\n\nAssignment:\n")
		if !found || strings.Contains(prompt, "Assignment:") || strings.Contains(prompt, task.Goal) {
			t.Fatal("task context leaked into the literal user prompt")
		}
		if role == codingagent.Review && (!strings.Contains(instruction, "Inspect only this task") || !strings.Contains(instruction, "sibling") ||
			!strings.Contains(instruction, "file list is context") || strings.Contains(instruction, "supplied read tool")) {
			t.Fatalf("%s prompt can escape its packet: %s", role, instruction)
		}
		if role == codingagent.Execute && (!strings.Contains(instruction, "Codex normally") || strings.Contains(instruction, "file tools") || strings.Contains(instruction, "receipt")) {
			t.Fatalf("executor retained custom tool or result instructions: %s", instruction)
		}
		var decoded struct {
			Task *struct {
				ID                   string                     `json:"id"`
				Goal                 string                     `json:"goal"`
				Guidance             []codingagent.TaskGuidance `json:"guidance"`
				KnownAffectedSystems []string                   `json:"known_affected_systems"`
				Acceptance           []string                   `json:"acceptance"`
				ReadFiles            []string                   `json:"read_files"`
				WriteFiles           []string                   `json:"write_files"`
				DeleteFiles          []string                   `json:"delete_files"`
				Checks               []codingagent.Check        `json:"checks"`
			} `json:"task"`
		}
		if err := json.Unmarshal([]byte(assignment), &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.Task == nil || decoded.Task.ID != task.ID || decoded.Task.Goal != task.Goal ||
			!reflect.DeepEqual(decoded.Task.Guidance, task.Guidance) ||
			!reflect.DeepEqual(decoded.Task.KnownAffectedSystems, task.KnownAffectedSystems) ||
			!reflect.DeepEqual(decoded.Task.Acceptance, task.Acceptance) ||
			!reflect.DeepEqual(decoded.Task.ReadFiles, task.ReadFiles) ||
			!reflect.DeepEqual(decoded.Task.WriteFiles, task.WriteFiles) ||
			!reflect.DeepEqual(decoded.Task.DeleteFiles, task.DeleteFiles) ||
			!reflect.DeepEqual(decoded.Task.Checks, task.Checks) {
			t.Fatalf("%s changed or expanded its task packet: %#v", role, decoded)
		}
		for _, internal := range []string{"internal-scheduler-edge", "internal-requirement", "internal-criterion", `"job_id"`, `"generation"`, `"root"`, `"route"`} {
			if strings.Contains(assignment, internal) {
				t.Fatalf("%s received code-owned scheduler or authority context %q: %s", role, internal, assignment)
			}
		}
	}
}

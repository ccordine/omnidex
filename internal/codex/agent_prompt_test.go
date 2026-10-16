package codex

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestPlanningOwnsBroadPictureAndCompilesTaskLocalPackets(t *testing.T) {
	for _, role := range []codingagent.Role{codingagent.PlanningInitial, codingagent.PlanningMutation} {
		call := codingagent.Assignment{Role: role}
		if role == codingagent.PlanningInitial {
			call.ReadFiles = []string{"AGENTS.md", "internal/runtime.go"}
		}
		prompt, err := renderAgentAssignment(call)
		if err != nil {
			t.Fatal(err)
		}
		instruction, assignment, _ := strings.Cut(prompt, "\n\nAssignment:\n")
		for _, boundary := range []string{"task", "requirement", "constraint", "acceptance", "file", "sibling"} {
			if !strings.Contains(strings.ToLower(instruction), boundary) {
				t.Fatalf("%s planning boundary lacks %q: %s", role, boundary, instruction)
			}
		}
		if role == codingagent.PlanningInitial && (!strings.Contains(assignment, `"workspace_files"`) || !strings.Contains(assignment, `"internal/runtime.go"`)) {
			t.Fatalf("initial planning did not receive the code-enumerated workspace picture: %s", assignment)
		}
	}
}

func TestPlanningJudgmentsCannotMutateOrExpandLedger(t *testing.T) {
	for _, role := range []codingagent.Role{codingagent.PlanningCoverage, codingagent.PlanningAudit} {
		prompt, err := renderAgentAssignment(codingagent.Assignment{Role: role})
		if err != nil {
			t.Fatal(err)
		}
		instruction, _, _ := strings.Cut(prompt, "\n\nAssignment:\n")
		if !strings.Contains(instruction, "read-only judgment") || !strings.Contains(instruction, "only a human-origin planning action may alter the ledger") {
			t.Fatalf("%s has autonomous ledger authority", role)
		}
	}
}

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
		instruction, assignment, _ := strings.Cut(prompt, "\n\nAssignment:\n")
		if !strings.Contains(instruction, "entire context") || !strings.Contains(instruction, "sibling") || strings.Contains(instruction, "full filesystem") {
			t.Fatalf("%s prompt can escape its packet: %s", role, instruction)
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

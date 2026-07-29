package worker

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestBoundedSpecialistRepairPromptLeadsWithDirectFailure(t *testing.T) {
	prompt, err := buildV3SpecialistRepairPrompt(
		json.RawMessage(`{"type":"object"}`),
		v3SpecialistInvocation{
			RoleID: "prompt_interpreter", ObjectiveID: "interpret", Objective: "Interpret one request",
			SuccessCriteria: []string{"Only grounded requirements remain"},
			Payload:         map[string]any{"current_user_instruction": "Build a task tracker"},
		},
		errors.New(`intent invented concrete path "pocket_tasks.go" that is absent from current authority; remove that path-bearing claim and do not substitute another path`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(prompt, "DIRECT CONTRACT CORRECTION") {
		t.Fatalf("repair prompt does not lead with failure: %s", prompt)
	}
	if strings.Index(prompt, "VALIDATION_FAILURE") > strings.Index(prompt, "CURRENT_AUTHORITY") {
		t.Fatal("repair prompt buried direct validation feedback behind authority context")
	}
	for _, required := range []string{"pocket_tasks.go", "Build a task tracker", "remove that path-bearing claim", "OUTPUT_SCHEMA"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("repair prompt omitted %q", required)
		}
	}
	for _, forbidden := range []string{"ROLE_RULES", "PREVIOUS_INVALID_RESPONSE", "MECHANICAL_EDIT_REQUIRED"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("repair prompt retained forbidden context %q", forbidden)
		}
	}
}

func TestSpecialistRepairLimitIsBounded(t *testing.T) {
	if got := v3SpecialistRepairLimit("bounded_repair_passes"); got != 3 {
		t.Fatalf("repair limit=%d, want 3", got)
	}
}

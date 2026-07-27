package worker

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/specialist"
)

func TestBoundedSpecialistRepairPromptLeadsWithDirectFailure(t *testing.T) {
	prompt, err := buildV3SpecialistRepairPrompt(
		json.RawMessage(`{"type":"object"}`),
		v3SpecialistInvocation{
			RoleID: "prompt_interpreter", ObjectiveID: "interpret", Objective: "Interpret one request",
			SuccessCriteria: []string{"Only grounded requirements remain"},
			Payload:         map[string]any{"current_user_instruction": "Build a task tracker"},
		},
		errors.New(`intent invented concrete path "pocket_tasks.go"`),
		`{"output":{"acceptance_criteria":["pocket_tasks.go exists"]}}`,
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
	for _, required := range []string{"pocket_tasks.go", "Repeating the rejected response is another failure", "Build a task tracker", "MECHANICAL_EDIT_REQUIRED", "Do not substitute a different concrete path"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("repair prompt omitted %q", required)
		}
	}
	if strings.Contains(prompt, "ROLE_RULES") {
		t.Fatal("repair prompt replayed the full role instructions instead of a minimal correction contract")
	}
}

func TestPromptInterpreterRepairEscalatesAcrossDistinctModels(t *testing.T) {
	runtime := &nativeRuntimeV3{svc: &Service{models: ModelRouting{
		Plan: "qwen3:4b-thinking", Analyze: "qwen2.5-coder:7b", Fast: "qwen2.5-coder:7b",
		Specialist: map[string]string{specialist.RolePlannerSpecialist: "qwen2.5-coder:14b"},
	}}}
	models := runtime.specialistRepairModels("prompt_interpreter", "qwen2.5-coder:7b", 3)
	want := []string{"qwen2.5-coder:7b", "qwen3:4b-thinking", "qwen2.5-coder:14b"}
	if strings.Join(models, "|") != strings.Join(want, "|") {
		t.Fatalf("repair models=%#v, want %#v", models, want)
	}
	if got := v3SpecialistRepairLimit("bounded_repair_passes"); got != 3 {
		t.Fatalf("repair limit=%d, want 3", got)
	}
}

func TestPromptInterpreterRepairSkipsDuplicatePlannerAndUsesReasoningModel(t *testing.T) {
	runtime := &nativeRuntimeV3{svc: &Service{models: ModelRouting{
		Plan: "qwen3:4b-thinking", Reasoning: "qwen2.5-coder:14b", Analyze: "qwen2.5-coder:7b", Fast: "qwen2.5-coder:7b",
		Specialist: map[string]string{specialist.RolePlannerSpecialist: "qwen3:4b-thinking"},
	}}}
	models := runtime.specialistRepairModels("prompt_interpreter", "qwen2.5-coder:7b", 3)
	want := []string{"qwen2.5-coder:7b", "qwen3:4b-thinking", "qwen2.5-coder:14b"}
	if strings.Join(models, "|") != strings.Join(want, "|") {
		t.Fatalf("repair models=%#v, want %#v", models, want)
	}
}

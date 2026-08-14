package worker

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/operation"
)

func TestDirectCodingCommandDiagnosticExcludesVolatileExecutionSummary(t *testing.T) {
	detail := directCodingCommandResult(operation.Result{
		Summary: "Command failed duration_ms=928",
		Output: map[string]any{
			"exit_code": 1,
			"stderr":    "main.go:3: undefined: run",
		},
	})
	if strings.Contains(detail, "duration") || strings.Contains(detail, "Command failed") {
		t.Fatalf("volatile summary leaked into diagnostic: %q", detail)
	}
	for _, required := range []string{"exit_code=1", "main.go:3: undefined: run"} {
		if !strings.Contains(detail, required) {
			t.Fatalf("diagnostic omitted %q: %q", required, detail)
		}
	}
}

func TestDirectCodingMutationSummaryReportsActualOperationsAndPaths(t *testing.T) {
	summary := renderDirectCodingMutationJournal([]directCodingMutationJournalEntry{
		{Path: "z.ts", Operation: workspaceFileDelete},
		{Path: "b.ts", Operation: workspaceFileCreate},
		{Path: "a.ts", Operation: workspaceFileCreate},
		{Path: "main.ts", Operation: workspaceFileReplace},
	})
	if summary != "created=[a.ts,b.ts] replaced=[main.ts] deleted=[z.ts]" {
		t.Fatalf("summary=%q", summary)
	}
}

func TestDirectCodingCompletionGatesOnlyFinalClaim(t *testing.T) {
	state := directCodingCompletionState{
		MutationCount: 1, LatestMutationTurn: 4, TestsRequired: true,
		WrittenSource: map[string]string{"main.go": "package main\n\nfunc main() {}\n"},
	}
	if err := validateDirectCodingCompletion(state); err == nil || !strings.Contains(err.Error(), "test command") {
		t.Fatalf("unverified final claim err=%v", err)
	}
	state.LatestCheckTurn = 5
	state.LatestTestTurn = 5
	if err := validateDirectCodingCompletion(state); err != nil {
		t.Fatalf("verified final claim rejected: %v", err)
	}

	state.WrittenSource["main.go"] = "package main\n\n// TODO implement\nfunc main() {}\n"
	if err := validateDirectCodingCompletion(state); err == nil || !strings.Contains(err.Error(), "unfinished") {
		t.Fatalf("unfinished final source err=%v", err)
	}
}

func TestDirectCodingCorrectionCanVerifyAcceptedWorkspaceWithoutNoOpRewrite(t *testing.T) {
	state := directCodingCompletionState{
		AllowExistingWorkspace: true,
		LatestCheckTurn:        3,
		WrittenSource:          map[string]string{},
	}
	if err := validateDirectCodingCompletion(state); err != nil {
		t.Fatalf("resumed verified workspace was rejected: %v", err)
	}
	state.AllowExistingWorkspace = false
	if err := validateDirectCodingCompletion(state); err == nil || !strings.Contains(err.Error(), "no source mutation") {
		t.Fatalf("fresh job completed without a mutation: %v", err)
	}
}

func TestDirectCodingCompletionRejectsNoTestSuccess(t *testing.T) {
	state := directCodingCompletionState{
		MutationCount: 1, LatestMutationTurn: 2, LatestCheckTurn: 3, LatestTestTurn: 3,
		TestsRequired: true, LastTestHadNoTests: true, WrittenSource: map[string]string{},
	}
	if err := validateDirectCodingCompletion(state); err == nil || !strings.Contains(err.Error(), "reported no tests") {
		t.Fatalf("no-test completion err=%v", err)
	}
}

func TestDirectCodingVerificationRejectsInspectionCommands(t *testing.T) {
	for _, command := range []struct {
		program string
		args    []string
	}{
		{program: "git", args: []string{"status"}},
		{program: "git", args: []string{"diff", "--no-ext-diff", "--no-textconv", "--"}},
		{program: "go", args: []string{"mod", "init", "example"}},
		{program: "npm", args: []string{"init", "--yes"}},
		{program: "npm", args: []string{"run", "dev"}},
		{program: "pnpm", args: []string{"run", "start"}},
		{program: "yarn", args: []string{"serve"}},
	} {
		if isDirectCodingVerificationCommand(command.program, command.args) {
			t.Fatalf("inspection/initializer counted as verification: %s %v", command.program, command.args)
		}
	}
	for _, command := range []struct {
		program string
		args    []string
	}{
		{program: "go", args: []string{"test", "./..."}},
		{program: "cargo", args: []string{"check"}},
		{program: "npm", args: []string{"test"}},
		{program: "npm", args: []string{"run", "lint"}},
		{program: "pnpm", args: []string{"typecheck"}},
		{program: "yarn", args: []string{"run", "test:unit"}},
	} {
		if !isDirectCodingVerificationCommand(command.program, command.args) {
			t.Fatalf("real verification was not counted: %s %v", command.program, command.args)
		}
	}
}

func TestDirectCodingReceivesOrderedFeedbackOnTheSameJob(t *testing.T) {
	runtime := &nativeRuntimeV3{claim: &model.ClaimedStep{
		Job: model.Job{ID: 41, Instruction: "Build the task application"},
		Contexts: []model.StepContext{
			{ID: 2, Key: "user_feedback", Value: "Keep the current domain API"},
			{ID: 3, Key: "user_feedback", Value: "Now correct the CLI error"},
		},
	}}
	request, err := runtime.directCodingRequest()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(request.Feedback, "|") != "Keep the current domain API|Now correct the CLI error" {
		t.Fatalf("feedback=%#v", request.Feedback)
	}
	if runtime.claim.Job.ID != 41 {
		t.Fatal("feedback handling replaced the active job")
	}
}

func TestDirectScrumCodingUsesCardInsteadOfSyntheticInstruction(t *testing.T) {
	runtime := &nativeRuntimeV3{claim: &model.ClaimedStep{Job: model.Job{
		Instruction: "Execute the authoritative Scrum card task.",
		Pipeline:    model.PipelineScrum,
		Metadata: json.RawMessage(`{
			"source":"omni-scrum",
			"project_id":7,
			"scrum_card_id":"card-7",
			"scrum_card_title":"Build the real Blade screen",
			"scrum_card_description":"Render tasks and validation errors",
			"scrum_checklist":"","scrum_test_criteria":"",
			"scrum_return_column":"assigned","scrum_channel_origin":false,
			"scrum_channel_operation_id":"","model_config":{},
			"telemetry_run_id":"00000000-0000-4000-8000-000000000001"
		}`),
	}}}
	request, err := runtime.directCodingRequest()
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"Build the real Blade screen", "Render tasks and validation errors"} {
		if !strings.Contains(request.Instruction, required) {
			t.Fatalf("Scrum instruction omitted %q: %s", required, request.Instruction)
		}
	}
	if strings.Contains(request.Instruction, "Execute the authoritative") {
		t.Fatalf("synthetic transport instruction reached coder: %s", request.Instruction)
	}
}

func TestDirectScrumChannelCodingUsesOnlyCurrentInstructionAsAuthority(t *testing.T) {
	runtime := &nativeRuntimeV3{claim: &model.ClaimedStep{Job: model.Job{
		Instruction: "Fix only the routing defect",
		Pipeline:    model.PipelineScrum,
		Metadata: json.RawMessage(`{
			"source":"omni-scrum",
			"project_id":8,
			"scrum_card_id":"card-8",
			"scrum_card_title":"Repair agent routing",
			"scrum_card_description":"Preserve the current API",
			"scrum_checklist":"","scrum_test_criteria":"",
			"scrum_return_column":"assigned","scrum_channel_origin":true,
			"scrum_channel_operation_id":"lifecycle_operation_0000000000000000000000000000000000000000000000000000000000000000",
			"model_config":{},
			"telemetry_run_id":"00000000-0000-4000-8000-000000000001"
		}`),
	}}}
	request, err := runtime.directCodingRequest()
	if err != nil {
		t.Fatal(err)
	}
	if request.Instruction != "Fix only the routing defect" {
		t.Fatalf("instruction=%q", request.Instruction)
	}
	if !strings.Contains(strings.Join(request.AdditionalAuthority, "\n"), "Preserve the current API") {
		t.Fatalf("authoritative card scope missing: %#v", request.AdditionalAuthority)
	}
}

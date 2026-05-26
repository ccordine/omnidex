package omni

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type rejectingChildReviewer struct {
	reason string
}

func (r rejectingChildReviewer) ReviewChildJob(input ChildJobReviewInput) ChildJobReviewerResult {
	return ChildJobReviewerResult{Accepted: false, Reviewer: "test_reviewer", Reason: r.reason}
}

func TestChildJobActiveJobDoesNotYieldToUnrelatedObjectiveAfterError(t *testing.T) {
	active := ChildJob{
		ID:                         "rename_app_js_to_jsx",
		ParentObjectiveID:          "repair_build",
		Goal:                       "rename App.js after JSX build failure",
		Status:                     ChildJobStatusRepairing,
		ScopeFiles:                 []string{"src/App.js", "src/App.jsx"},
		RequiredEvidencePredicates: []string{"file_exists:src/App.jsx", "file_absent:src/App.js"},
		LatestFailurePacket:        &FailurePacket{FailureKind: "missing_source_file"},
	}
	proposed := ChildJob{ID: "style_app", ParentObjectiveID: "polish_ui", Goal: "style unrelated app shell"}
	result := RunChildJobLoopOnce(ChildJobLoopInput{
		Jobs:              []ChildJob{active},
		WorkingDirectory:  t.TempDir(),
		ProposedParentJob: &proposed,
	})

	if result.ActiveJob == nil || result.ActiveJob.ID != active.ID {
		t.Fatalf("active child yielded to unrelated work: %#v", result)
	}
	if !result.PlannerBlocked || !result.ParentJobDeferred {
		t.Fatalf("parent planner should be blocked/deferred: %#v", result)
	}
	if len(result.Jobs) != 1 {
		t.Fatalf("unrelated parent job was enqueued while active child non-terminal: %#v", result.Jobs)
	}
}

func TestChildJobCompletesOnFirstPassWhenEvidenceAlreadySatisfiesPredicates(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "src", "App.jsx"), []byte("export default function App(){ return <main /> }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ledger := []StructuredObjective{{ID: "repair_build", Description: "Repair Vite build", Status: "pending"}}
	result := RunChildJobLoopOnce(ChildJobLoopInput{
		Jobs: []ChildJob{{
			ID:                         "rename_app_js_to_jsx",
			ParentObjectiveID:          "repair_build",
			Goal:                       "rename App.js after JSX build failure",
			Status:                     ChildJobStatusPending,
			RequiredEvidencePredicates: []string{"file_exists:src/App.jsx", "file_absent:src/App.js"},
		}},
		WorkingDirectory: workspace,
		ObjectiveLedger:  ledger,
	})

	if result.ActiveJob == nil || result.ActiveJob.Status != ChildJobStatusComplete {
		t.Fatalf("child job did not complete from existing evidence: %#v", result)
	}
	if got := result.ObjectiveLedger[0]; got.Status != "satisfied" || !strings.Contains(got.Evidence, "child_job_complete") {
		t.Fatalf("objective ledger not reconciled from completed child: %#v", result.ObjectiveLedger)
	}
	if !childJobEventsContain(result.Events, "workspace_route_refreshed_after_child_completion") {
		t.Fatalf("missing route refresh event: %#v", result.Events)
	}
}

func TestChildJobDoesNotSatisfyParentUntilSiblingChildrenComplete(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "src", "App.jsx"), []byte("export default function App(){ return <main /> }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ledger := []StructuredObjective{{ID: "complete_app", Description: "Complete app", Status: "pending"}}
	result := RunChildJobLoopOnce(ChildJobLoopInput{
		Jobs: []ChildJob{
			{
				ID:                         "write_app",
				ParentObjectiveID:          "complete_app",
				Status:                     ChildJobStatusPending,
				RequiredEvidencePredicates: []string{"file_exists:src/App.jsx"},
			},
			{
				ID:                         "verify_build",
				ParentObjectiveID:          "complete_app",
				Status:                     ChildJobStatusPending,
				RequiredEvidencePredicates: []string{"command_passed:npm run build"},
			},
		},
		WorkingDirectory: workspace,
		ObjectiveLedger:  ledger,
	})
	if result.Jobs[0].Status != ChildJobStatusComplete {
		t.Fatalf("first child should complete: %#v", result.Jobs)
	}
	if result.ObjectiveLedger[0].Status == "satisfied" {
		t.Fatalf("parent objective satisfied before sibling child completed: %#v", result.ObjectiveLedger)
	}
}

func TestChildJobParentPlannerNotCalledWhileNonTerminal(t *testing.T) {
	result := RunChildJobLoopOnce(ChildJobLoopInput{
		Jobs: []ChildJob{{
			ID:                         "verify_build",
			Status:                     ChildJobStatusActive,
			RequiredEvidencePredicates: []string{"command_passed:npm run build"},
		}},
		WorkingDirectory: t.TempDir(),
	})
	if !result.PlannerBlocked {
		t.Fatalf("non-terminal active child should block parent planner: %#v", result)
	}
	if result.ActiveJob == nil || result.ActiveJob.ID != "verify_build" {
		t.Fatalf("active child missing: %#v", result)
	}
}

func TestChildJobReviewerRejectionCreatesFocusedNextActionForSameChild(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "src", "App.jsx"), []byte("export default function App(){ return <main /> }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := RunChildJobLoopOnce(ChildJobLoopInput{
		Jobs: []ChildJob{{
			ID:                         "rename_app_js_to_jsx",
			Status:                     ChildJobStatusPending,
			ScopeFiles:                 []string{"src/App.jsx"},
			RequiredEvidencePredicates: []string{"file_exists:src/App.jsx"},
		}},
		WorkingDirectory: workspace,
		Reviewer:         rejectingChildReviewer{reason: "readback missing"},
	})

	if result.ActiveJob == nil || result.ActiveJob.ID != "rename_app_js_to_jsx" || result.ActiveJob.Status != ChildJobStatusRepairing {
		t.Fatalf("reviewer rejection did not keep focus on same child: %#v", result)
	}
	if result.NextAction == nil || result.NextAction.JobID != "rename_app_js_to_jsx" {
		t.Fatalf("missing focused next action for same child: %#v", result.NextAction)
	}
	if result.ActiveJob.LatestFailurePacket == nil || result.ActiveJob.LatestFailurePacket.FailureKind != "reviewer_rejected" {
		t.Fatalf("missing reviewer failure packet: %#v", result.ActiveJob)
	}
}

func TestChildCompletionRefreshesRouteFilesAndObjectiveLedger(t *testing.T) {
	completed := ChildJob{
		ID:                         "rename_app_js_to_jsx",
		ParentObjectiveID:          "repair_build",
		Status:                     ChildJobStatusComplete,
		RequiredEvidencePredicates: []string{"file_exists:src/App.jsx", "file_absent:src/App.js"},
	}
	route := RouteFilesAfterChildCompletion(TaskRoute{LikelyFiles: []string{"src/App.js", "src/main.jsx"}}, completed)
	if containsString(route.LikelyFiles, "src/App.js") || !containsString(route.LikelyFiles, "src/App.jsx") {
		t.Fatalf("route files not refreshed after child completion: %#v", route.LikelyFiles)
	}

	ledger := reconcileObjectiveLedgerFromCompletedChildJob([]StructuredObjective{{ID: "repair_build", Status: "pending"}}, completed)
	if ledger[0].Status != "satisfied" || !strings.Contains(ledger[0].Evidence, "rename_app_js_to_jsx") {
		t.Fatalf("objective ledger not refreshed: %#v", ledger)
	}
}

func TestFailedMoveCreatesFailurePacketWithMissingSourceFile(t *testing.T) {
	job := ChildJob{
		ID:                         "rename_app_js_to_jsx",
		ParentObjectiveID:          "repair_build",
		Status:                     ChildJobStatusActive,
		RequiredEvidencePredicates: []string{"file_exists:src/App.jsx", "file_absent:src/App.js"},
	}
	obs := StructuredCommandObservation{
		CommandID: "cmd_mv",
		Command:   "mv src/App.js src/App.jsx",
		ExitCode: 1,
		Stderr:   "mv: cannot stat 'src/App.js': No such file or directory",
		CWD:      "/repo/app",
	}
	job = AppendChildJobAttemptWithContext(job, obs, "shell", "shell_specialist", "qwen", "/repo/app")
	if len(job.AttemptLedger) != 1 {
		t.Fatalf("attempt ledger = %#v", job.AttemptLedger)
	}
	if job.LatestFailurePacket == nil || job.LatestFailurePacket.FailureKind != "missing_source_file" {
		t.Fatalf("failure packet = %#v", job.LatestFailurePacket)
	}
	if !containsString(job.LatestFailurePacket.ForbiddenNextActions, "mv src/App.js src/App.jsx") {
		t.Fatalf("failure packet missing forbidden repeat: %#v", job.LatestFailurePacket)
	}
}

func TestNextSpecialistPromptIncludesChildFailureAttemptHistory(t *testing.T) {
	ledger := []StructuredObjective{{
		ID:               "rename_app_js_to_jsx",
		Description:      "Rename App.js to App.jsx",
		Status:           "pending",
		Kind:             string(WorkItemKindUpdate),
		Required:         true,
		Source:           structuredObjectiveSourceUserExplicit,
		RequiredEvidence: []string{"file_exists:src/App.jsx", "file_absent:src/App.js"},
	}}
	obs := []StructuredCommandObservation{{
		CommandID: "cmd_mv",
		Command:   "mv src/App.js src/App.jsx",
		ExitCode: 1,
		Stderr:   "mv: cannot stat 'src/App.js': No such file or directory",
	}}
	message := buildStructuredCommandUserMessage("repair Vite JSX", obs, t.TempDir(), ledger, MinimalContext{}, nil, WorksiteSurvey{})
	if !strings.Contains(message, "attempt_ledger") || !strings.Contains(message, "mv src/App.js src/App.jsx") || !strings.Contains(message, "cannot stat") {
		t.Fatalf("planner payload missing failed attempt history:\n%s", message)
	}
}

func TestChildJobCannotRepeatFailedMove(t *testing.T) {
	job := ChildJob{ID: "rename_app_js_to_jsx", Status: ChildJobStatusRepairing}
	job = AppendChildJobAttempt(job, StructuredCommandObservation{
		Command:  "mv src/App.js src/App.jsx",
		ExitCode: 1,
		Stderr:   "mv: cannot stat src/App.js",
	}, "shell", "shell_specialist", "")
	if !ChildJobShouldRejectRepeat(job, "mv src/App.js src/App.jsx") {
		t.Fatalf("failed move should be rejected from attempt ledger: %#v", job)
	}
}

func TestChildJobLongStderrIsSummarizedWithOutputRef(t *testing.T) {
	long := strings.Repeat("compile failure line\n", 80)
	job := AppendChildJobAttempt(ChildJob{ID: "verify_build", Status: ChildJobStatusActive}, StructuredCommandObservation{
		CommandID: "cmd_build",
		Command:   "npm run build",
		ExitCode: 1,
		Stderr:   long,
	}, "shell", "shell_specialist", "")
	if job.LatestFailurePacket == nil {
		t.Fatalf("missing failure packet: %#v", job)
	}
	if len(job.LatestFailurePacket.StderrExcerpt) >= len(long) || !strings.Contains(job.LatestFailurePacket.StderrExcerpt, "[truncated]") {
		t.Fatalf("stderr not summarized: %#v", job.LatestFailurePacket.StderrExcerpt)
	}
	if job.LatestFailurePacket.FullOutputReferenceHint == "" || job.AttemptLedger[0].FullOutputRef == "" {
		t.Fatalf("missing full output ref: packet=%#v attempt=%#v", job.LatestFailurePacket, job.AttemptLedger[0])
	}
}

func TestRepeatedFailedCommandRejectedFromAttemptLedgerInStructuredRun(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"mv src/App.js src/App.jsx","done":false,"answer":""}`,
		`{"command":"mv src/App.js src/App.jsx","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"done"}`,
	}}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		ObjectiveLedger: []StructuredObjective{{
			ID:               "rename_app_js_to_jsx",
			Description:      "Rename App.js to App.jsx",
			Status:           "pending",
			Kind:             string(WorkItemKindUpdate),
			Required:         true,
			Source:           structuredObjectiveSourceUserExplicit,
			RequiredEvidence: []string{"file_exists:src/App.jsx", "file_absent:src/App.js"},
		}},
	}}}
	events := []StructuredCommandEvent{}
	result, _ := runStructuredCommandDecisionWithConfig(context.Background(), "repair Vite JSX", nil, client, &bytes.Buffer{}, &bytes.Buffer{}, func(evt StructuredCommandEvent) {
		events = append(events, evt)
	}, nil, structuredCommandDecisionRunConfig{
		CurrentWorkingDirectory: t.TempDir(),
		PromptInterpreter:      interpreter,
	})
	if !structuredEventsContain(events, "child_job_attempt_repeat_rejected") {
		t.Fatalf("missing repeat rejection event: events=%#v result=%#v", events, result)
	}
}

func childJobEventsContain(events []ChildJobEvent, eventType string) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

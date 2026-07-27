package omni

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeSourceVerificationCannotReplaceRequiredCompilerEvidence(t *testing.T) {
	runtimeOwned := StructuredCommandObservation{
		Command:           "runtime.source_verify zig",
		ExitCode:          0,
		EvidenceKind:      "source_verification",
		GeneratedBy:       "runtime",
		VerifierID:        "zig-source-verifier",
		CheckedFiles:      []string{"build.zig", "src/main.zig"},
		CheckedPredicates: []string{"file_nonempty:build.zig", "file_nonempty:src/main.zig"},
	}
	if !runtimeSourceVerificationObservation(runtimeOwned) {
		t.Fatal("expected runtime-owned source verification to remain valid artifact evidence")
	}
	objective := StructuredObjective{
		ID:               "compile_zig",
		Description:      "Compile the Zig application",
		Status:           "pending",
		Required:         true,
		RequiredEvidence: []string{"command_passed:zig build"},
	}
	if structuredObjectiveRequiredEvidenceSatisfied(objective, []StructuredCommandObservation{runtimeOwned}, t.TempDir()) {
		t.Fatal("source verification must not replace required compiler evidence")
	}
}

func TestValidateNestedGoModuleCommandScopeRejectsRootGoModInit(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "backend", "calculus-api"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "backend", "calculus-api", "go.mod"), []byte("module calculus-api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := validateStructuredCommandForRunWithSurvey("go mod init calculus", nil, dir, nil, WorksiteSurvey{})
	if err == nil {
		t.Fatal("expected root go mod init to be rejected when nested module exists")
	}
	if !strings.Contains(err.Error(), "nested module") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReconcileObjectiveLedgerSatisfiesRemovalObjective(t *testing.T) {
	ledger := []StructuredObjective{
		{ID: "remove_calculator_js", Description: "Remove src/calculator.js if it is empty and unused.", Status: "pending"},
		{ID: "run_npm_test", Description: "Run npm test after cleanup.", Status: "pending"},
	}
	events := []StructuredCommandEvent{}
	updated := reconcileStructuredObjectiveLedgerFromObservation(1, ledger, StructuredCommandObservation{
		Step:     1,
		Command:  "rm src/calculator.js && npm test",
		ExitCode: 0,
		Stdout:   "calculator smoke test passed",
	}, func(evt StructuredCommandEvent) {
		events = append(events, evt)
	})

	for _, id := range []string{"remove_calculator_js", "run_npm_test"} {
		found := false
		for _, objective := range updated {
			if objective.ID == id {
				found = true
				if !structuredObjectiveSatisfied(objective) {
					t.Fatalf("%s not satisfied: %#v", id, objective)
				}
			}
		}
		if !found {
			t.Fatalf("missing objective %s in %#v", id, updated)
		}
	}
	if !structuredEventsContain(events, "objective_ledger_reconciled") {
		t.Fatalf("missing reconciliation event: %#v", events)
	}
}

func TestReconcileObjectiveLedgerSatisfiesRenameWithFileEvidence(t *testing.T) {
	workspace := t.TempDir()
	srcDir := filepath.Join(workspace, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(srcDir, "App.js")
	if err := os.WriteFile(oldPath, []byte("export default function App(){ return <main /> }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := CommandDecisionResult{}
	if err := runStructuredPayloadCommand(context.Background(), 1, "mv src/App.js src/App.jsx", workspace, false, "", &bytes.Buffer{}, &bytes.Buffer{}, nil, &result); err != nil {
		t.Fatal(err)
	}
	ledger := []StructuredObjective{{
		ID:               "rename_app_js_to_jsx",
		Description:      "Rename App.js to App.jsx after Vite reported JSX in a .js file.",
		Status:           "pending",
		RequiredEvidence: []string{"file_exists:src/App.jsx", "file_absent:src/App.js"},
	}}
	updated := reconcileStructuredObjectiveLedgerFromObservation(1, ledger, result.Observations[0], nil)
	if !structuredObjectiveSatisfied(updated[0]) {
		t.Fatalf("rename objective not satisfied from file evidence: %#v", updated)
	}
}

func TestMutationReconciliationGateSuccessfulMoveMarksObjectiveComplete(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "src", "App.js"), []byte("export default function App(){ return <main /> }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := CommandDecisionResult{ObjectiveLedger: []StructuredObjective{{
		ID:               "rename_app_js_to_jsx",
		Description:      "Rename App.js to App.jsx after Vite reported JSX in a .js file.",
		Status:           "pending",
		RequiredEvidence: []string{"file_exists:src/App.jsx", "file_absent:src/App.js"},
	}}}
	events := []StructuredCommandEvent{}
	if err := runStructuredPayloadCommand(context.Background(), 1, "mv src/App.js src/App.jsx", workspace, false, "", &bytes.Buffer{}, &bytes.Buffer{}, func(evt StructuredCommandEvent) {
		events = append(events, evt)
	}, &result); err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("move failed: %#v", result.Observations)
	}
	if !structuredObjectiveSatisfied(result.ObjectiveLedger[0]) {
		t.Fatalf("objective not completed by mutation gate: %#v", result.ObjectiveLedger)
	}
	if fileExists(filepath.Join(workspace, "src", "App.js")) || !fileExists(filepath.Join(workspace, "src", "App.jsx")) {
		t.Fatalf("move state not reconciled")
	}
	for _, want := range []string{"file_move_verified", "workspace_route_refreshed_after_mutation"} {
		if !structuredEventsContain(events, want) {
			t.Fatalf("missing %s event: %#v", want, events)
		}
	}
}

func TestMutationReconciliationGateAlreadyMovedMarksObjectiveCompleteWithoutCommand(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "src", "App.jsx"), []byte("export default function App(){ return <main /> }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := CommandDecisionResult{ObjectiveLedger: []StructuredObjective{{
		ID:               "rename_app_js_to_jsx",
		Description:      "Rename App.js to App.jsx",
		Status:           "pending",
		RequiredEvidence: []string{"file_exists:src/App.jsx", "file_absent:src/App.js"},
	}}}
	events := []StructuredCommandEvent{}
	if err := runStructuredPayloadCommand(context.Background(), 1, "mv src/App.js src/App.jsx", workspace, false, "", &bytes.Buffer{}, &bytes.Buffer{}, func(evt StructuredCommandEvent) {
		events = append(events, evt)
	}, &result); err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("already reconciled move should be skipped as success: %#v", result.Observations)
	}
	if !strings.Contains(result.Observations[0].Stdout, "file_move_already_reconciled") {
		t.Fatalf("expected already reconciled evidence: %#v", result.Observations[0])
	}
	if !structuredObjectiveSatisfied(result.ObjectiveLedger[0]) {
		t.Fatalf("objective not completed by already reconciled move: %#v", result.ObjectiveLedger)
	}
	if !structuredEventsContain(events, "file_move_already_reconciled") {
		t.Fatalf("missing already reconciled event: %#v", events)
	}
}

func TestMutationReconciliationGateRefreshesRouteFilesAfterMove(t *testing.T) {
	route := taskRouteAfterMutationMove(TaskRoute{LikelyFiles: []string{"src/App.js", "src/main.jsx"}}, "src/App.js", "src/App.jsx")
	if containsString(route.LikelyFiles, "src/App.js") {
		t.Fatalf("route files still include old path: %#v", route.LikelyFiles)
	}
	if !containsString(route.LikelyFiles, "src/App.jsx") {
		t.Fatalf("route files missing new path: %#v", route.LikelyFiles)
	}
}

func TestMutationReconciliationGateRepeatedMoveSkippedAfterReconciliation(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "src", "App.js"), []byte("export default function App(){ return <main /> }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := CommandDecisionResult{}
	if err := runStructuredPayloadCommand(context.Background(), 1, "mv src/App.js src/App.jsx", workspace, false, "", &bytes.Buffer{}, &bytes.Buffer{}, nil, &result); err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("initial move failed: %#v", result.Observations)
	}
	if err := runStructuredPayloadCommand(context.Background(), 2, "mv src/App.js src/App.jsx", workspace, false, "", &bytes.Buffer{}, &bytes.Buffer{}, nil, &result); err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("repeated move should be skipped as already reconciled: %#v", result.Observations)
	}
	latest := result.Observations[len(result.Observations)-1]
	if !strings.Contains(latest.Stdout, "file_move_already_reconciled") {
		t.Fatalf("repeated move was not skipped: %#v", latest)
	}
}

func TestNoJSFilesWithJSXPredicate(t *testing.T) {
	workspace := t.TempDir()
	srcDir := filepath.Join(workspace, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "App.jsx"), []byte("export default function App(){ return <main /> }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ledger := []StructuredObjective{{
		ID:               "ensure_all_components_are_jsx",
		Description:      "Ensure .js files do not contain JSX.",
		Status:           "pending",
		RequiredEvidence: []string{"no_js_files_with_jsx:src"},
	}}
	updated := reconcileStructuredObjectiveLedgerFromObservation(1, ledger, StructuredCommandObservation{
		Step:     1,
		Command:  "grep -R \"<[A-Za-z]\" -n src --include=\"*.js\" || true",
		ExitCode: 0,
		CWD:      workspace,
	}, nil)
	if !structuredObjectiveSatisfied(updated[0]) {
		t.Fatalf("jsx extension objective not satisfied when no .js files contain JSX: %#v", updated)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "Widget.js"), []byte("export default function Widget(){ return <section /> }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	updated = reconcileStructuredObjectiveLedgerFromObservation(2, ledger, StructuredCommandObservation{
		Step:     2,
		Command:  "grep -R \"<[A-Za-z]\" -n src --include=\"*.js\" || true",
		ExitCode: 0,
		CWD:      workspace,
	}, nil)
	if structuredObjectiveSatisfied(updated[0]) {
		t.Fatalf("jsx extension objective should remain pending when .js contains JSX: %#v", updated)
	}
}

func TestViteJSXFeedbackCreatesRepairChildJobEvents(t *testing.T) {
	workspace := t.TempDir()
	binDir := filepath.Join(workspace, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	npmPath := filepath.Join(binDir, "npm")
	if err := os.WriteFile(npmPath, []byte("#!/bin/sh\necho '[plugin:vite:esbuild] src/App.js:10:4: ERROR: The JSX syntax extension is not currently enabled' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	result := CommandDecisionResult{}
	events := []StructuredCommandEvent{}
	command := shellQuote(npmPath) + " run build"
	_ = runStructuredPayloadCommand(context.Background(), 1, command, workspace, false, "", &bytes.Buffer{}, &bytes.Buffer{}, func(evt StructuredCommandEvent) {
		events = append(events, evt)
	}, &result)
	if !structuredEventsContain(events, "toolchain_feedback_classified") {
		t.Fatalf("missing toolchain feedback event: %#v", events)
	}
	if !structuredEventsContain(events, "toolchain_repair_child_job_created") {
		t.Fatalf("missing repair child job event: %#v", events)
	}
	if !structuredEventsContain(events, "toolchain_repair_focus_locked") {
		t.Fatalf("missing repair focus lock event: %#v", events)
	}
}

func TestToolchainFeedbackCreatesActiveRepairChildJob(t *testing.T) {
	workspace := t.TempDir()
	binDir := filepath.Join(workspace, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	npmPath := filepath.Join(binDir, "npm")
	if err := os.WriteFile(npmPath, []byte("#!/bin/sh\necho '[vite]: Rollup failed to resolve import \"react\" from src/App.jsx' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	result := CommandDecisionResult{}
	events := []StructuredCommandEvent{}
	command := "PATH=" + shellQuote(binDir) + ":$PATH npm run build"
	if err := runStructuredPayloadCommand(context.Background(), 1, command, workspace, false, "", &bytes.Buffer{}, &bytes.Buffer{}, func(evt StructuredCommandEvent) {
		events = append(events, evt)
	}, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.ChildJobs) != 1 || result.ChildJobs[0].Status != ChildJobStatusRepairing {
		t.Fatalf("toolchain feedback did not create active repair child job: %#v", result.ChildJobs)
	}
	if result.ChildJobs[0].LatestFailurePacket == nil || result.ChildJobs[0].LatestFailurePacket.FailureKind != "toolchain_failure" {
		t.Fatalf("missing toolchain failure packet: %#v", result.ChildJobs[0])
	}
	for _, want := range []string{"toolchain_feedback_classified", "toolchain_repair_child_job_created", "toolchain_repair_focus_locked"} {
		if !structuredEventsContain(events, want) {
			t.Fatalf("missing %s event: %#v", want, events)
		}
	}
}

func TestToolchainRepairFocusBlocksUnrelatedPlannerObjective(t *testing.T) {
	workspace := t.TempDir()
	binDir := filepath.Join(workspace, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	npmPath := filepath.Join(binDir, "npm")
	if err := os.WriteFile(npmPath, []byte("#!/bin/sh\necho '[vite]: Rollup failed to resolve import \"react\" from src/App.jsx' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	buildCommand := "PATH=" + shellQuote(binDir) + ":$PATH npm run build"
	client := &fakeCommandDecisionClient{responses: []string{
		fmt.Sprintf(`{"command":%q,"done":false,"answer":""}`, buildCommand),
		`{"command":"printf unrelated > unrelated.txt","done":false,"answer":""}`,
	}}
	events := []StructuredCommandEvent{}
	result, _ := runStructuredCommandDecisionWithConfig(context.Background(), "fix build then update docs", nil, client, &bytes.Buffer{}, &bytes.Buffer{}, func(evt StructuredCommandEvent) {
		events = append(events, evt)
	}, nil, structuredCommandDecisionRunConfig{
		CurrentWorkingDirectory: workspace,
		PromptInterpreter: &fakePromptInterpreter{interpretations: []PromptInterpretation{{
			ObjectiveLedger: []StructuredObjective{
				{ID: "repair_build", Description: "Repair frontend build", Status: "pending", Required: true},
				{ID: "update_docs", Description: "Update unrelated docs", Status: "pending", Required: true},
			},
		}}},
	})
	if client.calls != 1 {
		t.Fatalf("generic planner should not be called while repair child is active; calls=%d result=%#v", client.calls, result)
	}
	if fileExists(filepath.Join(workspace, "unrelated.txt")) {
		t.Fatalf("unrelated objective command ran while repair child active")
	}
	if !structuredEventsContain(events, "toolchain_repair_focus_lock_active") {
		t.Fatalf("missing active focus lock event: %#v", events)
	}
}

func TestFailingBuildFeedsFailurePacketIntoSameChildLoop(t *testing.T) {
	workspace := t.TempDir()
	binDir := filepath.Join(workspace, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	npmPath := filepath.Join(binDir, "npm")
	if err := os.WriteFile(npmPath, []byte("#!/bin/sh\necho '[vite]: Rollup failed to resolve import \"react\" from src/App.jsx' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	result := CommandDecisionResult{}
	command := "PATH=" + shellQuote(binDir) + ":$PATH npm run build"
	if err := runStructuredPayloadCommand(context.Background(), 1, command, workspace, false, "", &bytes.Buffer{}, &bytes.Buffer{}, nil, &result); err != nil {
		t.Fatal(err)
	}
	if err := runStructuredPayloadCommand(context.Background(), 2, command, workspace, false, "", &bytes.Buffer{}, &bytes.Buffer{}, nil, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.ChildJobs) != 1 {
		t.Fatalf("failing build should update same child job: %#v", result.ChildJobs)
	}
	job := result.ChildJobs[0]
	if len(job.AttemptLedger) == 0 {
		t.Fatalf("failure attempt not fed into child loop: %#v", job.AttemptLedger)
	}
	if job.LatestFailurePacket == nil || job.LatestFailurePacket.FailureKind != "toolchain_failure" {
		t.Fatalf("latest failure packet not retained from failing build: packet=%#v observations=%#v", job.LatestFailurePacket, result.Observations)
	}
}

func TestResearchOnlyReactPromptDoesNotMutateProjectFiles(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "package.json"), []byte(`{"name":"research-app","dependencies":{"react":"latest","react-dom":"latest"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	appPath := filepath.Join(workspace, "src", "App.jsx")
	original := "export default function App(){ return <main>original</main> }\n"
	if err := os.WriteFile(appPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf changed > src/App.jsx","done":false,"answer":""}`,
		`{"command":"cat package.json","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"researched React project metadata"}`,
	}}
	events := []StructuredCommandEvent{}
	result, err := runStructuredCommandDecisionWithConfig(context.Background(), "research React JS patterns in this project", nil, client, &bytes.Buffer{}, &bytes.Buffer{}, func(evt StructuredCommandEvent) {
		events = append(events, evt)
	}, nil, structuredCommandDecisionRunConfig{CurrentWorkingDirectory: workspace})
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(appPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != original {
		t.Fatalf("research-only command mutated source file: %q", string(content))
	}
	if result.TaskMode != TaskModeResearchOnly {
		t.Fatalf("task mode = %q, want research_only", result.TaskMode)
	}
	if !structuredEventsContain(events, "structured_command_rejected") && !structuredEventsContain(events, "research_only_mutation_rejected") {
		t.Fatalf("missing research-only rejection event: %#v", events)
	}
}

func TestResearchOnlyModeCanStoreExpertiseMemory(t *testing.T) {
	runner := newFakeMemoryRunner()
	store := NewPGMemoryStore(runner)
	record, err := store.AddMemory(context.Background(), "research", MemoryKindExpertise, "React research note: hooks must run unconditionally.", []string{"react", "expertise-memory", string(TaskModeResearchOnly)})
	if err != nil {
		t.Fatal(err)
	}
	if record.Kind != MemoryKindExpertise {
		t.Fatalf("stored memory kind = %q", record.Kind)
	}
	if len(runner.memories) != 1 {
		t.Fatalf("expertise memory was not stored: %#v", runner.memories)
	}
}

func TestToolchainErrorDuringResearchRecordedButNotRepaired(t *testing.T) {
	workspace := t.TempDir()
	binDir := filepath.Join(workspace, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	nodePath := filepath.Join(binDir, "node")
	if err := os.WriteFile(nodePath, []byte("#!/bin/sh\necho '[vite]: Rollup failed to resolve import \"react\" from src/App.jsx' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	result := CommandDecisionResult{TaskMode: TaskModeResearchOnly}
	events := []StructuredCommandEvent{}
	command := "PATH=" + shellQuote(binDir) + ":$PATH node -e \"console.error('vite failure'); process.exit(1)\""
	if err := runStructuredPayloadCommand(context.Background(), 1, command, workspace, false, "", &bytes.Buffer{}, &bytes.Buffer{}, func(evt StructuredCommandEvent) {
		events = append(events, evt)
	}, &result); err != nil {
		t.Fatal(err)
	}
	if !structuredEventsContain(events, "toolchain_feedback_classified") {
		t.Fatalf("missing classified feedback event: %#v", events)
	}
	if !structuredEventsContain(events, "toolchain_feedback_recorded_research_only") {
		t.Fatalf("missing research-only feedback recording event: %#v", events)
	}
	if len(result.ChildJobs) != 0 {
		t.Fatalf("research-only toolchain feedback created repair child jobs: %#v", result.ChildJobs)
	}
}

func TestStaleRepairChildJobsDoNotLeakIntoResearchOnlyTask(t *testing.T) {
	result := CommandDecisionResult{
		TaskMode: TaskModeResearchOnly,
		ChildJobs: []ChildJob{{
			ID:                "repair_vite_module_error",
			ParentObjectiveID: "toolchain_feedback_repair",
			Status:            ChildJobStatusRepairing,
		}},
	}
	events := []StructuredCommandEvent{}
	handled, err := routeActiveToolchainRepairChildBeforePlanner(context.Background(), 1, "research React JS", structuredCommandDecisionRunConfig{}, WorksiteSurvey{TaskMode: TaskModeResearchOnly}, &bytes.Buffer{}, &bytes.Buffer{}, func(evt StructuredCommandEvent) {
		events = append(events, evt)
	}, nil, &result)
	if err != nil {
		t.Fatal(err)
	}
	if handled {
		t.Fatalf("research-only task should not route into stale repair child")
	}
	if len(result.ChildJobs) != 0 {
		t.Fatalf("stale repair child job leaked into research-only task: %#v", result.ChildJobs)
	}
	if !structuredEventsContain(events, "research_only_repair_jobs_suppressed") {
		t.Fatalf("missing stale repair suppression event: %#v", events)
	}
}

func TestCommandObservationCarriesActiveChildJobOwner(t *testing.T) {
	workspace := t.TempDir()
	result := CommandDecisionResult{
		ObjectiveLedger: []StructuredObjective{{ID: "install_dependencies", Description: "Install dependencies", Status: "pending", Required: true}},
		ChildJobs: []ChildJob{{
			ID:                         "install_dependencies",
			Goal:                       "Install dependencies",
			Status:                     ChildJobStatusActive,
			RequiredEvidencePredicates: []string{"command_passed:true"},
		}},
	}
	events := []StructuredCommandEvent{}
	if err := runStructuredPayloadCommand(context.Background(), 1, "true", workspace, false, "", &bytes.Buffer{}, &bytes.Buffer{}, func(evt StructuredCommandEvent) {
		events = append(events, evt)
	}, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Observations) == 0 || result.Observations[0].ChildJobID != "install_dependencies" || result.Observations[0].ObjectiveID != "install_dependencies" {
		t.Fatalf("observation missing child ownership: %#v", result.Observations)
	}
	if !structuredEventsContain(events, "command_bound_to_child_job") {
		t.Fatalf("missing command binding event: %#v", events)
	}
}

func TestReconciliationUsesObservationChildJobNotDefaultInitializeNPM(t *testing.T) {
	workspace := t.TempDir()
	obs := StructuredCommandObservation{Step: 1, CommandID: "cmd_install", Command: "true", ChildJobID: "install_dependencies", ObjectiveID: "install_dependencies", ExitCode: 0, CWD: workspace}
	reconciled := RunSuccessReconciliation(SuccessReconciliationInput{
		LatestObservation: &obs,
		ObjectiveLedger: []StructuredObjective{
			{ID: "install_dependencies", Description: "Install dependencies", Status: "pending", Required: true},
		},
		ChildJobs: []ChildJob{
			{ID: "initialize_npm", Goal: "Initialize npm", Status: ChildJobStatusActive, RequiredEvidencePredicates: []string{"package_json_exists"}},
			{ID: "install_dependencies", Goal: "Install dependencies", Status: ChildJobStatusPending, RequiredEvidencePredicates: []string{"command_passed:true"}},
		},
		WorkingDirectory: workspace,
		Observations:     []StructuredCommandObservation{obs},
	})
	for _, job := range reconciled.ChildJobs {
		if job.ID == "initialize_npm" {
			t.Fatalf("stale initialize_npm remained selectable: %#v", reconciled.ChildJobs)
		}
	}
	if !successReconciliationEventsContain(reconciled.Events, "child_job_completed") {
		t.Fatalf("owned child job was not reconciled: %#v", reconciled.Events)
	}
}

func TestCompletedInitializeNPMRemovedFromChildQueue(t *testing.T) {
	jobs := SyncChildJobsWithObjectiveLedger([]ChildJob{
		{ID: "initialize_npm", Goal: "Initialize npm", Status: ChildJobStatusComplete},
		{ID: "install_dependencies", Goal: "Install dependencies", Status: ChildJobStatusPending},
	}, []StructuredObjective{
		{ID: "initialize_npm", Description: "Initialize npm", Status: "satisfied", Required: true},
		{ID: "install_dependencies", Description: "Install dependencies", Status: "pending", Required: true},
	})
	if len(jobs) != 1 || jobs[0].ID != "install_dependencies" {
		t.Fatalf("completed initialize_npm not removed from active queue: %#v", jobs)
	}
}

func TestReconciliationSkipsChildQueueMutationWhenOwnerMissing(t *testing.T) {
	workspace := t.TempDir()
	obs := StructuredCommandObservation{Step: 1, CommandID: "cmd", Command: "true", ExitCode: 0, CWD: workspace}
	reconciled := RunSuccessReconciliation(SuccessReconciliationInput{
		LatestObservation: &obs,
		ObjectiveLedger:   []StructuredObjective{{ID: "install_dependencies", Description: "Install dependencies", Status: "pending", Required: true}},
		ChildJobs:         []ChildJob{{ID: "install_dependencies", Goal: "Install dependencies", Status: ChildJobStatusActive, RequiredEvidencePredicates: []string{"command_passed:true"}}},
		WorkingDirectory:  workspace,
		Observations:      []StructuredCommandObservation{obs},
	})
	if !successReconciliationEventsContain(reconciled.Events, "reconciliation_skipped_missing_owner") {
		t.Fatalf("missing skipped reconciliation event: %#v", reconciled.Events)
	}
	if len(reconciled.ChildJobs) != 1 || reconciled.ChildJobs[0].Status != ChildJobStatusActive {
		t.Fatalf("missing-owner reconciliation mutated child queue: %#v", reconciled.ChildJobs)
	}
}

func TestScaffoldTargetRootPromotionAfterCreateReactApp(t *testing.T) {
	workspace := t.TempDir()
	binDir := filepath.Join(workspace, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	npxPath := filepath.Join(binDir, "npx")
	if err := os.WriteFile(npxPath, []byte("#!/bin/sh\nname=\"$2\"\nmkdir -p \"$name/src\" \"$name/node_modules\"\nprintf '{\"scripts\":{\"start\":\"react-scripts start\"}}' > \"$name/package.json\"\nprintf 'import App from \"./App\"\\n' > \"$name/src/index.js\"\nprintf 'export default function App(){return null}\\n' > \"$name/src/App.js\"\nprintf 'created %s\\n' \"$name\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	result := CommandDecisionResult{}
	events := []StructuredCommandEvent{}
	command := "PATH=" + shellQuote(binDir) + ":$PATH npx create-react-app note-app && cd note-app"
	if err := runStructuredPayloadCommand(context.Background(), 1, command, workspace, false, "", &bytes.Buffer{}, &bytes.Buffer{}, func(evt StructuredCommandEvent) {
		events = append(events, evt)
	}, &result); err != nil {
		t.Fatal(err)
	}
	wantRoot := filepath.Join(workspace, "note-app")
	if result.TargetRoot != wantRoot {
		t.Fatalf("target root = %q, want %q", result.TargetRoot, wantRoot)
	}
	if result.Command != "PATH="+shellQuote(binDir)+":$PATH npx create-react-app note-app" {
		t.Fatalf("trailing cd was not normalized out: %q", result.Command)
	}
	for _, rel := range []string{"package.json", "src/index.js", "src/App.js"} {
		if !fileExists(filepath.Join(wantRoot, rel)) {
			t.Fatalf("missing scaffold file %s", rel)
		}
	}
	for _, want := range []string{"scaffold_project_detected", "target_root_promoted_after_scaffold", "workspace_route_refreshed_after_mutation"} {
		if !structuredEventsContain(events, want) {
			t.Fatalf("missing %s event: %#v", want, events)
		}
	}
}

func TestCreateReactAppScaffoldEvidenceSatisfiesInitialChildJobs(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, "note-app")
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	for rel, body := range map[string]string{
		"package.json":      `{"scripts":{"start":"react-scripts start"}}`,
		"package-lock.json": "{}",
		"src/index.js":      "import App from './App'\n",
		"src/App.js":        "export default function App(){ return null }\n",
	} {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	command := "npx create-react-app note-app"
	obs := StructuredCommandObservation{
		Step:        1,
		CommandID:   "cmd_cra",
		ChildJobID:  "initialize_npm",
		ObjectiveID: "initialize_npm",
		Command:     command,
		ExitCode:    0,
		Stdout:      "new_target_root: " + root,
		CWD:         workspace,
	}
	reconciled := RunSuccessReconciliation(SuccessReconciliationInput{
		LatestObservation: &obs,
		ObjectiveLedger: []StructuredObjective{
			{ID: "initialize_npm", Description: "Initialize npm", Status: "pending", Required: true},
			{ID: "install_dependencies", Description: "Install dependencies", Status: "pending", Required: true},
			{ID: "create_entrypoint", Description: "Create entrypoint", Status: "pending", Required: true},
			{ID: "setup_note_component", Description: "Set up note component", Status: "pending", Required: true},
		},
		ChildJobs: []ChildJob{
			{ID: "initialize_npm", Goal: "Initialize npm", Status: ChildJobStatusActive, RequiredEvidencePredicates: []string{"package_json_exists"}},
			{ID: "install_dependencies", Goal: "Install dependencies", Status: ChildJobStatusPending, RequiredEvidencePredicates: []string{"dependencies_declared_or_installed"}},
			{ID: "create_entrypoint", Goal: "Create entrypoint", Status: ChildJobStatusPending, RequiredEvidencePredicates: []string{"entrypoint_exists"}},
			{ID: "setup_note_component", Goal: "Set up note component", Status: ChildJobStatusPending, RequiredEvidencePredicates: []string{"file_exists:src/Note.jsx"}},
		},
		WorkingDirectory: root,
		Observations:     []StructuredCommandObservation{obs},
	})
	for _, stale := range []string{"initialize_npm", "install_dependencies", "create_entrypoint"} {
		for _, job := range reconciled.ChildJobs {
			if job.ID == stale {
				t.Fatalf("%s should have been satisfied and removed: %#v", stale, reconciled.ChildJobs)
			}
		}
	}
	if reconciled.NextRequiredChildJob == nil || reconciled.NextRequiredChildJob.ID != "setup_note_component" {
		t.Fatalf("next child job = %#v, want setup_note_component", reconciled.NextRequiredChildJob)
	}
}

package omni

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFutureCommandRunsInsidePromotedScaffoldRoot(t *testing.T) {
	workspace := t.TempDir()
	binDir := filepath.Join(workspace, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	npxPath := filepath.Join(binDir, "npx")
	if err := os.WriteFile(npxPath, []byte("#!/bin/sh\nname=\"$2\"\nmkdir -p \"$name/src\"\nprintf '{}' > \"$name/package.json\"\nprintf 'x' > \"$name/src/App.js\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	createCommand := "npx create-react-app note-app"
	client := &fakeCommandDecisionClient{responses: []string{
		fmt.Sprintf(`{"command":%q,"done":false,"answer":""}`, createCommand),
		`{"command":"pwd","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"done"}`,
	}}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		ObjectiveLedger: []StructuredObjective{{
			ID:               "verify_promoted_root",
			Description:      "Verify commands run in the promoted root",
			Status:           "pending",
			Source:           structuredObjectiveSourceUserExplicit,
			Required:         true,
			RequiredEvidence: []string{"command_passed:pwd"},
		}},
	}}}
	events := []StructuredCommandEvent{}
	result, err := runStructuredCommandDecisionWithConfig(context.Background(), "verify the promoted command root", nil, client, &bytes.Buffer{}, &bytes.Buffer{}, func(event StructuredCommandEvent) {
		events = append(events, event)
	}, nil, structuredCommandDecisionRunConfig{
		CurrentWorkingDirectory: workspace,
		PromptInterpreter:       interpreter,
	})
	if err != nil {
		t.Fatalf("%v; event types=%v; observations=%#v", err, structuredEventTypesForTest(events), result.Observations)
	}
	if len(result.Observations) < 2 {
		t.Fatalf("missing observations: %#v", result.Observations)
	}
	second := result.Observations[1]
	wantRoot := filepath.Join(workspace, "note-app")
	if second.CWD != wantRoot || !strings.Contains(second.Stdout, wantRoot) {
		t.Fatalf("future command did not run in promoted root: obs=%#v want=%q", second, wantRoot)
	}
}

func TestReconcileObjectiveLedgerDoesNotSatisfyAllVerificationFromIrrelevantGoTest(t *testing.T) {
	ledger := []StructuredObjective{
		{ID: "verify_backend_tests", Description: "Verify backend Go tests.", Status: "pending"},
		{ID: "verify_frontend_tests", Description: "Verify frontend tests.", Status: "pending"},
		{ID: "run_smoke_test", Description: "Run smoke test.", Status: "pending"},
		{ID: "verify_frontend_build", Description: "Verify frontend build.", Status: "pending"},
	}
	updated := reconcileStructuredObjectiveLedgerFromObservation(1, ledger, StructuredCommandObservation{
		Step:     1,
		Command:  "go test ./...",
		ExitCode: 0,
		Stdout:   "?   \tcalculus/frontend/calculus-frontend/node_modules/flatted/golang/pkg/flatted\t[no test files]",
	}, nil)

	for _, objective := range updated {
		if structuredObjectiveSatisfied(objective) {
			t.Fatalf("objective %s should not be satisfied by irrelevant root go test output: %#v", objective.ID, updated)
		}
	}
}

func TestReconcileObjectiveLedgerSatisfiesSpecificGoReactVerificationEvidence(t *testing.T) {
	ledger := []StructuredObjective{
		{ID: "verify_backend_tests", Description: "Verify backend Go tests.", Status: "pending"},
		{ID: "verify_frontend_tests", Description: "Verify frontend tests.", Status: "pending"},
		{ID: "run_smoke_test", Description: "Run smoke test.", Status: "pending"},
		{ID: "verify_frontend_build", Description: "Verify frontend build.", Status: "pending"},
	}
	updated := reconcileStructuredObjectiveLedgerFromObservation(1, ledger, StructuredCommandObservation{
		Step:     1,
		Command:  "make test && make build",
		ExitCode: 0,
		Stdout:   "cd backend/calculus-api && go test ./...\nok  \tcalculus-api\t0.002s\nreact-scripts test --watchAll=false\nPASS src/App.test.js\ngo react calculus smoke test passed\nCompiled successfully.",
	}, nil)

	for _, objective := range updated {
		if !structuredObjectiveSatisfied(objective) {
			t.Fatalf("objective %s should be satisfied by make verification evidence: %#v", objective.ID, updated)
		}
	}
}

func TestReconcileObjectiveLedgerRequiresDockerLifecycleEvidence(t *testing.T) {
	ledger := []StructuredObjective{
		{ID: "create_dockerfile", Description: "Create Dockerfile", Status: "pending"},
		{ID: "build_docker_image", Description: "Build Docker image", Status: "pending"},
		{ID: "run_application_in_docker_container", Description: "Run application in Docker container", Status: "pending"},
	}
	afterDockerfile := reconcileStructuredObjectiveLedgerFromObservation(1, ledger, StructuredCommandObservation{
		Step:     1,
		Command:  "echo 'FROM nginx:alpine' > Dockerfile",
		ExitCode: 0,
		Stdout:   "Dockerfile created successfully.",
	}, nil)
	for _, objective := range afterDockerfile {
		if objective.ID != "create_dockerfile" && structuredObjectiveSatisfied(objective) {
			t.Fatalf("Dockerfile-only command should not satisfy lifecycle objective %s: %#v", objective.ID, afterDockerfile)
		}
	}

	afterLifecycle := reconcileStructuredObjectiveLedgerFromObservation(2, afterDockerfile, StructuredCommandObservation{
		Step:     2,
		Command:  "docker build -t app:test . && docker run -d --name app-test --restart=no -p 127.0.0.1:8080:80 app:test && curl -fsS http://127.0.0.1:8080/health && docker inspect -f '{{.State.Running}} {{.State.Restarting}} {{.RestartCount}}' app-test && docker logs app-test",
		ExitCode: 0,
		Stdout:   "Successfully built abc123\nrunning=true restarting=false restart_count=0\nhealth=ok\nDOCKER_LOGS_CLEAR",
	}, nil)
	for _, id := range []string{"build_docker_image", "run_application_in_docker_container"} {
		found := false
		for _, objective := range afterLifecycle {
			if objective.ID == id {
				found = true
				if !structuredObjectiveSatisfied(objective) {
					t.Fatalf("%s should be satisfied by lifecycle evidence: %#v", id, afterLifecycle)
				}
			}
		}
		if !found {
			t.Fatalf("missing objective %s", id)
		}
	}
}

func TestStructuredCommandDecisionAcceptsPartialCompletionAndContinues(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"go version","done":false,"answer":""}`,
		`{"command":"go env GOVERSION","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"build and test passed"}`,
	}}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		ObjectiveLedger: []StructuredObjective{
			{ID: "verify_build", Description: "Verify Go executable", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true, RequiredEvidence: []string{"command_passed:go version"}},
			{ID: "verify_test", Description: "Verify Go environment", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true, RequiredEvidence: []string{"command_passed:go env GOVERSION"}},
		},
	}}}
	checker := &fakeCompletionChecker{checks: []CompletionCheck{{
		Done:   false,
		Reason: "build passed but tests remain",
		ObjectiveLedger: []StructuredObjective{
			{ID: "verify_build", Description: "Verify build", Status: "satisfied", Evidence: "npm run build exited 0"},
			{ID: "verify_test", Description: "Verify test", Status: "pending"},
		},
	}, {
		Done:   true,
		Reason: "build and test passed",
		ObjectiveLedger: []StructuredObjective{
			{ID: "verify_build", Description: "Verify build", Status: "satisfied", Evidence: "npm run build exited 0"},
			{ID: "verify_test", Description: "Verify test", Status: "satisfied", Evidence: "npm test exited 0"},
		},
	}}}
	events := []StructuredCommandEvent{}
	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"verify app",
		nil,
		client,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		nil,
		structuredCommandDecisionRunConfig{
			PromptInterpreter: interpreter,
			CompletionChecker: checker,
		},
	)
	if err != nil {
		t.Fatalf("%v; event types=%v; observations=%#v", err, structuredEventTypesForTest(events), result.Observations)
	}
	if result.Command != "go env GOVERSION" {
		t.Fatalf("final command = %q, want continued test command", result.Command)
	}
	if len(checker.inputs) != 0 {
		t.Fatalf("deterministic typed evidence should complete without a model-only done check: %#v", checker.inputs)
	}
	if pending := pendingStructuredObjectives(result.ObjectiveLedger); len(pending) != 0 {
		t.Fatalf("ledger still pending: %#v", result.ObjectiveLedger)
	}
}

func TestCompletionCheckerCannotSatisfyObjectiveWithoutDeterministicEvidence(t *testing.T) {
	ledger := []StructuredObjective{{
		ID:               "verify_build",
		Description:      "Verify build",
		Status:           "pending",
		Source:           structuredObjectiveSourceUserExplicit,
		Required:         true,
		RequiredEvidence: []string{"command_passed:npm run build"},
	}}
	checker := &fakeCompletionChecker{checks: []CompletionCheck{{
		Done:   true,
		Reason: "looks complete",
		ObjectiveLedger: []StructuredObjective{{
			ID:       "verify_build",
			Status:   "satisfied",
			Evidence: "validator said build passed",
		}},
	}}}
	events := []StructuredCommandEvent{}
	result := runCompletionCheckDetailed(
		context.Background(),
		3,
		"build the app",
		t.TempDir(),
		ledger,
		MinimalContext{},
		nil,
		"done",
		checker,
		WorksiteSurvey{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
	)
	if result.Accepted {
		t.Fatal("completion checker must not accept objective claims without deterministic evidence")
	}
	if pending := pendingStructuredObjectives(result.Ledger); len(pending) != 1 {
		t.Fatalf("objective should remain pending, got %#v", result.Ledger)
	}
	if !structuredEventsContain(events, "completion_check_claim_rejected_for_missing_evidence") {
		t.Fatalf("missing claim rejection event: %#v", events)
	}
}

func TestCompletionCheckerFailureIsReturnedWithoutFallback(t *testing.T) {
	checker := &fakeCompletionChecker{errors: []error{errors.New("completion model unavailable")}}
	events := []StructuredCommandEvent{}
	result := runCompletionCheckDetailed(
		context.Background(),
		3,
		"verify the build",
		t.TempDir(),
		[]StructuredObjective{{
			ID:               "verify_build",
			Description:      "Verify build",
			Status:           "pending",
			Kind:             string(WorkItemKindVerify),
			Source:           structuredObjectiveSourceUserExplicit,
			Required:         true,
			RequiredEvidence: []string{"command_passed:npm run build"},
		}},
		MinimalContext{},
		nil,
		"done",
		checker,
		WorksiteSurvey{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
	)
	if result.Err == nil || !strings.Contains(result.Err.Error(), "completion model unavailable") {
		t.Fatalf("error = %v, want explicit completion-checker failure", result.Err)
	}
	if result.Accepted {
		t.Fatal("failed completion checker accepted completion")
	}
	if !structuredEventsContain(events, "completion_check_failed") {
		t.Fatalf("missing completion-checker failure event: %#v", events)
	}
}

func TestPlannerCannotSatisfyObjectiveWithoutDeterministicEvidence(t *testing.T) {
	ledger := []StructuredObjective{{
		ID:               "write_notes_app",
		Description:      "Write the notes app",
		Status:           "pending",
		Source:           structuredObjectiveSourceUserExplicit,
		Required:         true,
		RequiredEvidence: []string{"file_contains:src/App.js:Notes"},
	}}
	events := []StructuredCommandEvent{}
	updated := mergePlannerObjectiveLedger(2, ledger, []StructuredObjective{{
		ID:       "write_notes_app",
		Status:   "satisfied",
		Evidence: "planner says the file was written",
	}}, nil, t.TempDir(), func(event StructuredCommandEvent) {
		events = append(events, event)
	})
	if len(pendingStructuredObjectives(updated)) != 1 {
		t.Fatalf("planner claim bypassed deterministic evidence: %#v", updated)
	}
	if !structuredEventsContain(events, "planner_objective_claim_rejected_for_missing_evidence") {
		t.Fatalf("missing planner claim rejection event: %#v", events)
	}
}

func TestCompletionCheckerClaimAcceptedWhenRequiredEvidencePassed(t *testing.T) {
	ledger := []StructuredObjective{{
		ID:               "verify_build",
		Description:      "Verify build",
		Status:           "pending",
		Source:           structuredObjectiveSourceUserExplicit,
		Required:         true,
		RequiredEvidence: []string{"command_passed:npm run build"},
	}}
	checker := &fakeCompletionChecker{checks: []CompletionCheck{{
		Done:   true,
		Reason: "build evidence exists",
		ObjectiveLedger: []StructuredObjective{{
			ID:       "verify_build",
			Status:   "satisfied",
			Evidence: "npm run build exited 0",
		}},
	}}}
	result := runCompletionCheckDetailed(
		context.Background(),
		3,
		"build the app",
		t.TempDir(),
		ledger,
		MinimalContext{},
		[]StructuredCommandObservation{{Command: "npm run build", ExitCode: 0, Stdout: "built in 1s"}},
		"done",
		checker,
		WorksiteSurvey{},
		nil,
	)
	if !result.Accepted {
		t.Fatalf("completion checker claim should be accepted after required evidence passed: %#v", result.Ledger)
	}
}

func quoteJSONForTest(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return `"` + replacer.Replace(value) + `"`
}

func TestStructuredCommandRequestIncludesTypedWorkQueue(t *testing.T) {
	req := buildStructuredCommandRequestWithContextRecipesSurveyAndPrepRaw(
		"build a React notes app",
		nil,
		nil,
		nil,
		t.TempDir(),
		[]StructuredObjective{{
			ID:          "complete_notes_app",
			Description: "Complete notes app implementation",
			Status:      "pending",
			Kind:        string(WorkItemKindArchitect),
			Source:      structuredObjectiveSourceUserExplicit,
			Required:    true,
		}},
		MinimalContext{},
		nil,
		WorksiteSurvey{Frameworks: []string{"react"}, PackageManager: packageManagerNPM},
		PrepContextBundle{},
	)
	activeTask := activeTaskJSONForTest(t, req.Messages[len(req.Messages)-1].Content)
	for _, want := range []string{`"work_items"`, `"current_work_item"`, `"kind":"architect"`, `"kind":"create"`, `"scope"`, `"paths":["package.json"]`} {
		if !strings.Contains(activeTask, want) {
			t.Fatalf("active task missing %q: %s", want, activeTask)
		}
	}
}

func seedReactArchitectFileEvidence(t *testing.T, workspace string, contract ImplementationArchitectContract, paths ...string) {
	t.Helper()
	for _, rel := range paths {
		rel = filepath.ToSlash(rel)
		var content string
		switch strings.ToLower(rel) {
		case "package.json":
			content = deterministicReactPackageJSON(contract)
		case "vite.config.js":
			content = deterministicViteReactConfig()
		case "index.html":
			content = deterministicReactIndexHTML("src/main.jsx", contract)
		case "src/main.jsx":
			content = deterministicReactMountEntry("src/main.jsx")
		case "scripts/smoke-test.mjs":
			content = deterministicReactSmokeTest(contract)
		default:
			t.Fatalf("unsupported seed path %q", rel)
		}
		target := filepath.Join(workspace, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func structuredEventsContain(events []StructuredCommandEvent, eventType string) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

func structuredEventTypesForTest(events []StructuredCommandEvent) []string {
	types := make([]string, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
}

func structuredEventOfTypeForTest(events []StructuredCommandEvent, eventType string) *StructuredCommandEvent {
	for i := range events {
		if events[i].Type == eventType {
			return &events[i]
		}
	}
	return nil
}

func sameStringSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	gotSet := map[string]int{}
	for _, value := range got {
		gotSet[value]++
	}
	for _, value := range want {
		if gotSet[value] == 0 {
			return false
		}
		gotSet[value]--
	}
	return true
}

func activeTaskJSONForTest(t *testing.T, raw string) string {
	t.Helper()
	var payload struct {
		ActiveTask json.RawMessage `json:"active_task"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("decode active payload: %v\n%s", err, raw)
	}
	if len(payload.ActiveTask) == 0 {
		t.Fatalf("missing active_task: %s", raw)
	}
	return string(payload.ActiveTask)
}

package omni

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSuccessReconciliationSuccessfulMoveSatisfiesRenameObjective(t *testing.T) {
	workspace := t.TempDir()
	srcDir := filepath.Join(workspace, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "App.js"), []byte("export default function App(){ return <main /> }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := CommandDecisionResult{}
	if err := runStructuredPayloadCommand(context.Background(), 1, "mv src/App.js src/App.jsx", workspace, false, "", &bytes.Buffer{}, &bytes.Buffer{}, nil, &result); err != nil {
		t.Fatal(err)
	}
	ledger := []StructuredObjective{renameAppObjective()}
	reconciled := RunSuccessReconciliation(SuccessReconciliationInput{
		LatestObservation: &result.Observations[0],
		ObjectiveLedger:   ledger,
		WorkingDirectory:  workspace,
		Observations:      result.Observations,
	})
	if len(reconciled.SatisfiedObjectives) != 1 || reconciled.SatisfiedObjectives[0] != "rename_app_js_to_jsx" {
		t.Fatalf("rename objective not satisfied: %#v", reconciled)
	}
	if got := reconciled.ObjectiveLedger[0]; got.Status != "satisfied" {
		t.Fatalf("ledger status = %#v", reconciled.ObjectiveLedger)
	}
	if !successReconciliationEventsContain(reconciled.Events, "evidence_predicate_passed") ||
		!successReconciliationEventsContain(reconciled.Events, "objective_satisfied_from_evidence") {
		t.Fatalf("missing evidence events: %#v", reconciled.Events)
	}
}

func TestSuccessReconciliationSatisfiedFromFilesystemWithoutRerunningMove(t *testing.T) {
	workspace := t.TempDir()
	srcDir := filepath.Join(workspace, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "App.jsx"), []byte("export default function App(){ return <main /> }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reconciled := RunSuccessReconciliation(SuccessReconciliationInput{
		ObjectiveLedger:  []StructuredObjective{renameAppObjective()},
		WorkingDirectory: workspace,
	})
	if got := reconciled.ObjectiveLedger[0]; got.Status != "satisfied" {
		t.Fatalf("existing filesystem evidence did not satisfy rename: %#v", reconciled)
	}
}

func TestSuccessReconciliationNPMBuildProofSatisfiesObjective(t *testing.T) {
	ledger := []StructuredObjective{{
		ID:               "npm_build_proof",
		Description:      "Run npm build proof",
		Status:           "pending",
		Required:         true,
		Source:           structuredObjectiveSourceUserExplicit,
		RequiredEvidence: []string{"command_passed:npm run build"},
	}}
	obs := StructuredCommandObservation{CommandID: "cmd_build", Command: "npm run build", ExitCode: 0, Stdout: "built in 1s"}
	reconciled := RunSuccessReconciliation(SuccessReconciliationInput{
		LatestObservation: &obs,
		ObjectiveLedger:   ledger,
		WorkingDirectory:  t.TempDir(),
		Observations:      []StructuredCommandObservation{obs},
	})
	if got := reconciled.ObjectiveLedger[0]; got.Status != "satisfied" {
		t.Fatalf("build proof did not satisfy objective: %#v", reconciled)
	}
}

func TestSuccessReconciliationCompletedObjectiveRemovedFromPending(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "src", "App.jsx"), []byte("export default function App(){ return <main /> }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reconciled := RunSuccessReconciliation(SuccessReconciliationInput{
		ObjectiveLedger:  []StructuredObjective{renameAppObjective()},
		WorkingDirectory: workspace,
	})
	if pending := pendingStructuredObjectiveIDs(reconciled.ObjectiveLedger); pending != "" {
		t.Fatalf("completed objective still pending: %s ledger=%#v", pending, reconciled.ObjectiveLedger)
	}
}

func TestStructuredRunDoesNotCallPlannerBeforeSuccessReconciliation(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "src", "App.jsx"), []byte("export default function App(){ return <main /> }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := &fakeCommandDecisionClient{responses: []string{`{"command":"printf should_not_run","done":false,"answer":""}`}}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		ObjectiveLedger: []StructuredObjective{renameAppObjective()},
	}}}
	events := []StructuredCommandEvent{}
	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"repair Vite JSX filename",
		nil,
		client,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		nil,
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: workspace,
			PromptInterpreter:       interpreter,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 0 {
		t.Fatalf("planner was called before reconciliation closed objective: calls=%d", client.calls)
	}
	if result.Command != "SUCCESS_RECONCILIATION" || result.ExitCode != 0 {
		t.Fatalf("result = %#v", result)
	}
	if !structuredEventsContain(events, "success_reconciliation_started") ||
		!structuredEventsContain(events, "success_reconciliation_completed") {
		t.Fatalf("missing reconciliation events: %#v", events)
	}
}

func renameAppObjective() StructuredObjective {
	return StructuredObjective{
		ID:               "rename_app_js_to_jsx",
		Description:      "Rename App.js to App.jsx after Vite reported JSX in a .js file.",
		Status:           "pending",
		Required:         true,
		Source:           structuredObjectiveSourceUserExplicit,
		RequiredEvidence: []string{"file_exists:src/App.jsx", "file_absent:src/App.js"},
	}
}

func successReconciliationEventsContain(events []SuccessReconciliationEvent, eventType string) bool {
	for _, event := range events {
		if event.Type == eventType || strings.Contains(event.Type, eventType) {
			return true
		}
	}
	return false
}

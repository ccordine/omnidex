package omni

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindEmptyProjectFilesSkipsGeneratedDirs(t *testing.T) {
	root := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(root, "src"), 0o755))
	must(os.MkdirAll(filepath.Join(root, "node_modules", "pkg"), 0o755))
	must(os.WriteFile(filepath.Join(root, "src", "App.test.js"), nil, 0o644))
	must(os.WriteFile(filepath.Join(root, "node_modules", "pkg", "index.js"), nil, 0o644))

	files := findEmptyProjectFiles(root, 10)
	if len(files) != 1 || files[0] != "src/App.test.js" {
		t.Fatalf("files = %v", files)
	}
}

func TestEnforceNoEmptyProjectFilesBeforeCompletionAddsBlockingObjective(t *testing.T) {
	root := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(root, "src"), 0o755))
	must(os.WriteFile(filepath.Join(root, "src", "App.jsx"), nil, 0o644))

	ledger := []StructuredObjective{{
		ID:          "create_app",
		Description: "Create app",
		Status:      "satisfied",
		Source:      structuredObjectiveSourceUserExplicit,
		Required:    true,
	}}
	result := CommandDecisionResult{}
	events := []StructuredCommandEvent{}

	updated := enforceNoEmptyProjectFilesBeforeCompletion(
		3,
		"build a React app",
		root,
		ledger,
		[]StructuredCommandObservation{{Command: "npm run build", ExitCode: 0}},
		func(evt StructuredCommandEvent) {
			events = append(events, evt)
		},
		&result,
	)

	if len(pendingStructuredObjectives(updated)) == 0 {
		t.Fatalf("expected blocking objective: %#v", updated)
	}
	if !strings.Contains(pendingStructuredObjectiveIDs(updated), emptyProjectFileObjectiveID) {
		t.Fatalf("pending = %s", pendingStructuredObjectiveIDs(updated))
	}
	if len(result.Observations) != 1 || !strings.Contains(result.Observations[0].Stderr, "empty project file") {
		t.Fatalf("observations = %#v", result.Observations)
	}
	if !structuredEventsContain(events, "completion_check_rejected_empty_files") {
		t.Fatalf("events = %#v", events)
	}
}

func TestArtifactValidationGateRejectsPlaceholderOnlySource(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "App.jsx"), []byte("placeholder\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := CommandDecisionResult{ExitCode: 0}
	events := []StructuredCommandEvent{}
	rejected := rejectArtifactValidationGate(
		4,
		"build a React app",
		root,
		[]StructuredObjective{{ID: "create_app", Description: "Create app", Status: "satisfied", Source: structuredObjectiveSourceUserExplicit, Required: true}},
		[]StructuredCommandObservation{{Command: "cat > src/App.jsx", ExitCode: 0}},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		&result,
	)
	if !rejected {
		t.Fatal("expected placeholder-only source to fail artifact validation")
	}
	if result.ExitCode == 0 {
		t.Fatal("artifact validation failure should mark result failed")
	}
	if len(result.Observations) != 1 || result.Observations[0].EvidenceKind != "artifact_validation" || result.Observations[0].GeneratedBy != "runtime" {
		t.Fatalf("missing structured artifact validation observation: %#v", result.Observations)
	}
	if !structuredEventsContain(events, "artifact_validation_failed") {
		t.Fatalf("events = %#v", events)
	}
}

func TestArtifactValidationGateAddsRuntimePassEvidence(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "App.jsx"), []byte("export default function App(){ return <main>Notes</main>; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := CommandDecisionResult{ExitCode: 0}
	rejected := rejectArtifactValidationGate(
		5,
		"build a React app",
		root,
		[]StructuredObjective{{ID: "create_app", Description: "Create app", Status: "satisfied", Source: structuredObjectiveSourceUserExplicit, Required: true}},
		[]StructuredCommandObservation{{Command: "cat > src/App.jsx", ExitCode: 0}},
		nil,
		&result,
	)
	if rejected {
		t.Fatal("valid source should pass artifact validation")
	}
	if len(result.Observations) != 1 || result.Observations[0].EvidenceKind != "artifact_validation" || result.Observations[0].ExitCode != 0 {
		t.Fatalf("missing artifact validation pass observation: %#v", result.Observations)
	}
}

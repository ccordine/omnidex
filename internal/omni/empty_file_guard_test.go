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

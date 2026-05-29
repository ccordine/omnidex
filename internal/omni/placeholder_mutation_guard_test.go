package omni

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTouchTargetsProjectSourceArtifactRejectsJSAndHTML(t *testing.T) {
	for _, command := range []string{
		"touch src/index.js",
		"touch src/App.js",
		"touch index.html",
	} {
		if !touchTargetsProjectSourceArtifact(command) {
			t.Fatalf("expected %q to target a project source artifact", command)
		}
	}
	if touchTargetsProjectSourceArtifact("mkdir -p src/components") {
		t.Fatal("mkdir-only command should not count as touch source artifact")
	}
}

func TestValidatePlaceholderOnlySourceMutationRejectsTouchIndexJS(t *testing.T) {
	err := validatePlaceholderOnlySourceMutation("touch src/index.js", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "empty source files") {
		t.Fatalf("expected touch rejection, got %v", err)
	}
}

func TestValidateStructuredCommandForRunRejectsTouchSourceFile(t *testing.T) {
	err := validateStructuredCommandForRun(
		"touch src/index.js",
		nil,
		t.TempDir(),
		[]StructuredObjective{{ID: "create_app", Description: "Create the app entrypoint", Status: "pending"}},
	)
	if err == nil || !strings.Contains(err.Error(), "empty source files") {
		t.Fatalf("expected structured command rejection, got %v", err)
	}
}

func TestClassifyPlaceholderOnlyMutationAsFailure(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "src", "index.js")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	exitCode, reason := classifyPlaceholderOnlyMutationAsFailure("touch src/index.js", workspace, 0)
	if exitCode != 1 || !strings.Contains(reason, "partial_failure") {
		t.Fatalf("expected partial failure classification, got exit=%d reason=%q", exitCode, reason)
	}
}

func TestValidateConflictingEntrypointMutationRejectsSecondIndexHTML(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "index.html"), []byte("<!doctype html><html></html>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := validateConflictingEntrypointMutation("touch src/index.html", workspace)
	if err == nil || !strings.Contains(err.Error(), "duplicate html entrypoint") {
		t.Fatalf("expected duplicate index.html rejection, got %v", err)
	}
}

func TestProgressionGateForcesRecoveryAfterTouchSuccess(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "src", "index.js")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	decision := ProgressionGate{}.ReviewStep(ProgressionInput{
		Prompt:     "Build a JavaScript app",
		WorkingDir: workspace,
		ObjectiveLedger: []StructuredObjective{{
			ID:          "create_app_entrypoint",
			Description: "Create the JavaScript app entrypoint",
			Status:      "pending",
		}},
		Observations: []StructuredCommandObservation{{
			Step:     1,
			Command:  "touch src/index.js",
			ExitCode: 0,
		}},
	})
	if decision.Action != ProgressForceRecovery {
		t.Fatalf("expected forced recovery after placeholder touch, got %#v", decision)
	}
}

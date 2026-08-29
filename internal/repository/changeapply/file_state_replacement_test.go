package changeapply_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/gryph/omnidex/internal/repository/changeapply"
)

func TestPlanFileStateTransitionsStagesOnlyExactReplacementPostImagesAndAppliesThem(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t, map[string]fixtureEntry{
		"go.mod":        {content: "module example.com/replacement\n\ngo 1.24\n", mode: 0o600},
		"first.go":      {content: "package replacement\n\nfunc First() int { return 1 }\n", mode: 0o640},
		"second.go":     {content: "package replacement\n\nfunc Second() int { return 2 }\n", mode: 0o600},
		"retained.go":   {content: "package replacement\n\nfunc Retained() int { return 3 }\n", mode: 0o600},
		"unchanged.txt": {content: "exact retained bytes\n", mode: 0o644},
	})
	firstContent := []byte("package replacement\n\nfunc First() int { return 11 }\n")
	secondContent := []byte("package replacement\n\nfunc Second() int { return 22 }\n")
	first := replacementState(fixture.file(t, "first.go"), firstContent)
	second := replacementState(fixture.file(t, "second.go"), secondContent)
	retainedFile := fixture.file(t, "retained.go")
	retained := replacementState(
		retainedFile,
		[]byte("package replacement\n\nfunc Retained() int { return 3 }\n"),
	)
	packageID := packageArtifactID(t, fixture.analysis, "replacement")
	first.PackageArtifactID = packageID
	second.PackageArtifactID = packageID
	retained.PackageArtifactID = packageID

	stage, err := changeapply.PlanFileStateTransitions(t.Context(), changeapply.FileStateInput{
		Snapshot: fixture.snapshot, Analysis: fixture.analysis, OwnerID: "objective_replace_exact",
		Desired: []changeapply.DesiredFileState{second, retained, first},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Cleanup() })
	secondStage, err := changeapply.PlanFileStateTransitions(t.Context(), changeapply.FileStateInput{
		Snapshot: fixture.snapshot, Analysis: fixture.analysis, OwnerID: "objective_replace_exact",
		Desired: []changeapply.DesiredFileState{retained, first, second},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = secondStage.Cleanup() })

	if stage.Patch() != secondStage.Patch() || stage.PatchSHA256() != secondStage.PatchSHA256() ||
		stage.ID() != secondStage.ID() {
		t.Fatalf("replacement patch is not deterministic\nfirst:\n%s\nsecond:\n%s", stage.Patch(), secondStage.Patch())
	}
	firstHeader := strings.Index(stage.Patch(), "--- a/first.go\n+++ b/first.go")
	secondHeader := strings.Index(stage.Patch(), "--- a/second.go\n+++ b/second.go")
	if firstHeader < 0 || secondHeader <= firstHeader {
		t.Fatalf("replacement patch does not have deterministic path order:\n%s", stage.Patch())
	}
	if strings.Contains(stage.Patch(), "retained.go") {
		t.Fatalf("replacement patch contains already-exact file:\n%s", stage.Patch())
	}
	assertFile(t, filepath.Join(stage.DeltaRoot(), "first.go"), string(firstContent), 0o640)
	assertFile(t, filepath.Join(stage.DeltaRoot(), "second.go"), string(secondContent), 0o600)
	for _, unchanged := range []string{"go.mod", "retained.go", "unchanged.txt", ".git", ".omni"} {
		if _, err := os.Lstat(filepath.Join(stage.DeltaRoot(), unchanged)); !os.IsNotExist(err) {
			t.Fatalf("replacement delta contains unchanged path %q: %v", unchanged, err)
		}
	}

	states := stage.ExpectedFiles()
	statesByPath := make(map[string]changeapply.ExpectedFileState, len(states))
	for _, state := range states {
		statesByPath[state.Path] = state
	}
	firstState := statesByPath["first.go"]
	secondState := statesByPath["second.go"]
	if len(states) != 2 || firstState.Mode != 0o640 ||
		firstState.Size != int64(len(firstContent)) || firstState.SHA256 == first.Source.SHA256 ||
		secondState.Mode != 0o600 || secondState.Size != int64(len(secondContent)) ||
		secondState.SHA256 == second.Source.SHA256 {
		t.Fatalf("replacement expected states=%+v", states)
	}
	result, err := stage.ApplyVerified(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 2 || result.Files[0].Action != "update" || result.Files[0].Path != "first.go" ||
		result.Files[1].Action != "update" || result.Files[1].Path != "second.go" {
		t.Fatalf("replacement apply result=%+v", result.Files)
	}
	assertFile(t, filepath.Join(fixture.root, "first.go"), string(firstContent), 0o640)
	assertFile(t, filepath.Join(fixture.root, "second.go"), string(secondContent), 0o600)
	assertFile(t, filepath.Join(fixture.root, "retained.go"), string(retained.Content), os.FileMode(retainedFile.Mode))
	assertFile(t, filepath.Join(fixture.root, "unchanged.txt"), "exact retained bytes\n", 0o644)
}

func TestPlanFileStateTransitionsRejectsAnEntirelyAlreadyExactSet(t *testing.T) {
	t.Parallel()
	fixture := basicFixture(t)
	file := fixture.file(t, "first.go")
	exact := replacementState(
		file,
		[]byte("package changeapply\n\nfunc First() int { return 1 }\n"),
	)
	_, err := changeapply.PlanFileStateTransitions(t.Context(), changeapply.FileStateInput{
		Snapshot: fixture.snapshot, Analysis: fixture.analysis, OwnerID: "objective_zero_delta",
		Desired: []changeapply.DesiredFileState{
			exact,
			{Path: "already_absent.go", Present: false},
		},
	})
	if err == nil || err.Error() != "repository desired file states are already exact and require no mutation" {
		t.Fatalf("all-exact desired state error=%v", err)
	}
}

func TestPlanFileStateTransitionsRejectsReplacementSourceDriftAndInvalidAuthority(t *testing.T) {
	t.Parallel()
	fixture := basicFixture(t)
	file := fixture.file(t, "first.go")
	desired := replacementState(
		file,
		[]byte("package changeapply\n\nfunc First() int { return 2 }\n"),
	)
	invalid := desired
	invalid.Source.SHA256 = strings.Repeat("0", 64)
	if _, err := changeapply.PlanFileStateTransitions(t.Context(), changeapply.FileStateInput{
		Snapshot: fixture.snapshot, Analysis: fixture.analysis, OwnerID: "objective_invalid_authority",
		Desired: []changeapply.DesiredFileState{invalid},
	}); err == nil || !strings.Contains(err.Error(), "source authority") {
		t.Fatalf("invalid replacement authority error=%v", err)
	}

	drifted := "package changeapply\n\nfunc First() int { return 7 }\n"
	if err := os.WriteFile(filepath.Join(fixture.root, file.Path), []byte(drifted), os.FileMode(file.Mode)); err != nil {
		t.Fatal(err)
	}
	if _, err := changeapply.PlanFileStateTransitions(t.Context(), changeapply.FileStateInput{
		Snapshot: fixture.snapshot, Analysis: fixture.analysis, OwnerID: "objective_source_drift",
		Desired: []changeapply.DesiredFileState{desired},
	}); err == nil || (!strings.Contains(err.Error(), "changed") && !strings.Contains(err.Error(), "stale")) {
		t.Fatalf("replacement source drift error=%v", err)
	}
	assertFile(t, filepath.Join(fixture.root, file.Path), drifted, os.FileMode(file.Mode))
}

func TestPlanFileStateTransitionsRejectsReplacementDriftBeforeApply(t *testing.T) {
	t.Parallel()
	fixture := basicFixture(t)
	file := fixture.file(t, "first.go")
	desiredContent := []byte("package changeapply\n\nfunc First() int { return 2 }\n")
	stage, err := changeapply.PlanFileStateTransitions(t.Context(), changeapply.FileStateInput{
		Snapshot: fixture.snapshot, Analysis: fixture.analysis, OwnerID: "objective_apply_drift",
		Desired: []changeapply.DesiredFileState{replacementState(file, desiredContent)},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Cleanup() })

	drifted := "package changeapply\n\nfunc First() int { return 7 }\n"
	if err := os.WriteFile(filepath.Join(fixture.root, file.Path), []byte(drifted), os.FileMode(file.Mode)); err != nil {
		t.Fatal(err)
	}
	if _, err := stage.ApplyVerified(t.Context()); err == nil ||
		(!strings.Contains(err.Error(), "changed") && !strings.Contains(err.Error(), "stale")) {
		t.Fatalf("replacement apply drift error=%v", err)
	}
	assertFile(t, filepath.Join(fixture.root, file.Path), drifted, os.FileMode(file.Mode))
	assertFile(t, filepath.Join(stage.DeltaRoot(), file.Path), string(desiredContent), os.FileMode(file.Mode))
}

func replacementState(file repositoryfacts.File, content []byte) changeapply.DesiredFileState {
	return changeapply.DesiredFileState{
		Path: file.Path, Present: true,
		Source: changeapply.ExactSourceFile{
			FileID: file.ID, SHA256: file.SHA256, Size: file.Size, Mode: file.Mode,
		},
		Content: append([]byte(nil), content...), Mode: file.Mode,
	}
}

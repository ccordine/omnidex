package workspace_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/workspace"
)

func TestPlanMutationExpectedStateIncludesTrackedFileDeletionExclusion(t *testing.T) {
	root := trackedWorkspaceFixture(t)
	source, err := workspace.Capture(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := workspace.PlanMutation(
		t.Context(), source, "objective_"+strings.Repeat("2", 64),
		[]workspace.DesiredFileState{{
			Path: "tracked.txt", Source: workspaceSource(t, source, "tracked.txt"),
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := workspace.StageMutation(t.Context(), source, plan)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Cleanup() })
	if _, err := stage.ApplyVerified(t.Context()); err != nil {
		t.Fatal(err)
	}
	after, err := workspace.Capture(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.VerifyExpected(after); err != nil {
		t.Fatal(err)
	}
	if !workspaceSnapshotExcludes(after, "tracked.txt") {
		t.Fatalf("tracked deletion post-state lacks absent exclusion: %+v", after.Exclusions)
	}
}

func TestPlanMutationExpectedStateRemovesRestoredTrackedFileExclusion(t *testing.T) {
	root := trackedWorkspaceFixture(t)
	if err := os.Remove(filepath.Join(root, "tracked.txt")); err != nil {
		t.Fatal(err)
	}
	source, err := workspace.Capture(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !workspaceSnapshotExcludes(source, "tracked.txt") {
		t.Fatalf("missing tracked file was not captured as excluded: %+v", source.Exclusions)
	}
	plan, err := workspace.PlanMutation(
		t.Context(), source, "objective_"+strings.Repeat("3", 64),
		[]workspace.DesiredFileState{{
			Path: "tracked.txt", Present: true, Content: []byte("restored\n"), Mode: 0o644,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := workspace.StageMutation(t.Context(), source, plan)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Cleanup() })
	if _, err := stage.ApplyVerified(t.Context()); err != nil {
		t.Fatal(err)
	}
	after, err := workspace.Capture(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.VerifyExpected(after); err != nil {
		t.Fatal(err)
	}
	if workspaceSnapshotExcludes(after, "tracked.txt") {
		t.Fatalf("restored tracked file retained exclusion: %+v", after.Exclusions)
	}
}

func trackedWorkspaceFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeWorkspaceFile(t, root, "tracked.txt", "before\n", 0o644)
	workspaceGit(t, root, "init")
	workspaceGit(t, root, "config", "user.name", "Omnidex Test")
	workspaceGit(t, root, "config", "user.email", "omnidex@example.invalid")
	workspaceGit(t, root, "add", "tracked.txt")
	workspaceGit(t, root, "commit", "-m", "fixture")
	return root
}

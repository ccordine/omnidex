package workspace_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/workspace"
)

func TestStageMutationContainsOnlyChangedPostimagesAndAppliesVerified(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, "unchanged/large.txt", strings.Repeat("u", 32*1024)+"\n", 0o600)
	writeWorkspaceFile(t, root, "replace/value.txt", "before\n", 0o600)
	writeWorkspaceFile(t, root, "delete/value.txt", "delete\n", 0o640)
	source, err := workspace.Capture(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := workspace.PlanMutation(t.Context(), source, "objective_"+strings.Repeat("e", 64), []workspace.DesiredFileState{
		{Path: "replace/value.txt", Source: workspaceSource(t, source, "replace/value.txt"), Present: true, Content: []byte("after\n"), Mode: 0o600},
		{Path: "delete/value.txt", Source: workspaceSource(t, source, "delete/value.txt")},
		{Path: "created/value.txt", Present: true, Content: []byte("created\n"), Mode: 0o644},
	})
	if err != nil {
		t.Fatal(err)
	}
	stage, err := workspace.StageMutation(t.Context(), source, plan)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := stage.Cleanup(); err != nil {
			t.Error(err)
		}
	})
	if err := stage.VerifyExactDelta(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"created/value.txt", "replace/value.txt"} {
		if _, err := os.Stat(filepath.Join(stage.DeltaRoot(), filepath.FromSlash(path))); err != nil {
			t.Fatalf("expected delta postimage %q: %v", path, err)
		}
	}
	for _, path := range []string{"delete/value.txt", "unchanged/large.txt"} {
		if _, err := os.Stat(filepath.Join(stage.DeltaRoot(), filepath.FromSlash(path))); !os.IsNotExist(err) {
			t.Fatalf("delta unexpectedly contains %q: %v", path, err)
		}
	}
	result, err := stage.ApplyVerified(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 3 {
		t.Fatalf("apply result=%+v", result)
	}
	after, err := workspace.Capture(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.VerifyExpected(after); err != nil {
		t.Fatal(err)
	}
	if _, err := stage.ApplyVerified(t.Context()); err == nil || !strings.Contains(err.Error(), "already applied") {
		t.Fatalf("second apply error=%v", err)
	}
}

func TestStageMutationRejectsDeltaTamperAndSourceDrift(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, "value.txt", "before\n", 0o600)
	source, err := workspace.Capture(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := workspace.PlanMutation(t.Context(), source, "objective_"+strings.Repeat("f", 64), []workspace.DesiredFileState{{
		Path: "value.txt", Source: workspaceSource(t, source, "value.txt"),
		Present: true, Content: []byte("after\n"), Mode: 0o600,
	}})
	if err != nil {
		t.Fatal(err)
	}
	stage, err := workspace.StageMutation(t.Context(), source, plan)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Cleanup() })
	writeWorkspaceFile(t, stage.DeltaRoot(), "value.txt", "tampered\n", 0o600)
	if err := stage.VerifyExactDelta(t.Context()); err == nil || !strings.Contains(err.Error(), "tampered") {
		t.Fatalf("delta tamper error=%v", err)
	}
	if err := stage.Cleanup(); err != nil {
		t.Fatal(err)
	}
	stage, err = workspace.StageMutation(t.Context(), source, plan)
	if err != nil {
		t.Fatal(err)
	}
	writeWorkspaceFile(t, root, "value.txt", "drifted\n", 0o600)
	if _, err := stage.ApplyVerified(t.Context()); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("source drift error=%v", err)
	}
}

func TestStageMutationDoesNotDuplicateLargeSourceTree(t *testing.T) {
	root := t.TempDir()
	for index := 0; index < 3_100; index++ {
		writeWorkspaceFile(t, root, filepath.Join("large", "tree", strings.Repeat("x", 4), string(rune('a'+index%26)),
			strings.Repeat("n", 4)+"-"+fmtIndex(index)+".txt"), "unchanged\n", 0o600)
	}
	source, err := workspace.Capture(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := workspace.PlanMutation(t.Context(), source, "objective_"+strings.Repeat("1", 64), []workspace.DesiredFileState{{
		Path: "new/value.txt", Present: true, Content: []byte("new\n"), Mode: 0o644,
	}})
	if err != nil {
		t.Fatal(err)
	}
	stage, err := workspace.StageMutation(t.Context(), source, plan)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Cleanup() })
	regular := 0
	if err := filepath.WalkDir(stage.DeltaRoot(), func(_ string, entry os.DirEntry, err error) error {
		if err == nil && entry.Type().IsRegular() {
			regular++
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if regular != 1 {
		t.Fatalf("delta regular files=%d, expected one changed postimage", regular)
	}
}

func fmtIndex(index int) string {
	return fmt.Sprintf("%04d", index)
}

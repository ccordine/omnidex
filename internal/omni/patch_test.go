package omni

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyUnifiedPatchDoesNotPartiallyWriteWhenLaterFileIsInvalid(t *testing.T) {
	workspace := t.TempDir()
	first := filepath.Join(workspace, "first.txt")
	second := filepath.Join(workspace, "second.txt")
	if err := os.WriteFile(first, []byte("old first\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("old second\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	patch := "diff --git a/first.txt b/first.txt\n--- a/first.txt\n+++ b/first.txt\n@@ -1 +1 @@\n-old first\n+new first\n" +
		"diff --git a/second.txt b/second.txt\n--- a/second.txt\n+++ b/second.txt\n@@ -1 +1 @@\n-not the current content\n+new second\n"
	if _, err := ApplyUnifiedPatch(PatchApplyOptions{Workspace: workspace, Patch: patch}); err == nil {
		t.Fatal("invalid second file must reject the complete patch")
	}
	assertPatchFile(t, first, "old first\n", 0o640)
	assertPatchFile(t, second, "old second\n", 0o600)
}

func TestApplyUnifiedPatchRejectsDuplicateTargetWithoutWriting(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "same.txt")
	if err := os.WriteFile(target, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	patch := "diff --git a/same.txt b/same.txt\n--- a/same.txt\n+++ b/same.txt\n@@ -1 +1 @@\n-old\n+first\n" +
		"diff --git a/same.txt b/same.txt\n--- a/same.txt\n+++ b/same.txt\n@@ -1 +1 @@\n-old\n+second\n"
	if _, err := ApplyUnifiedPatch(PatchApplyOptions{Workspace: workspace, Patch: patch}); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate target err=%v", err)
	}
	assertPatchFile(t, target, "old\n", 0o644)
}

func TestApplyUnifiedPatchHonorsCanceledAuthorityContext(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "routing.txt")
	if err := os.WriteFile(target, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	patch := "diff --git a/routing.txt b/routing.txt\n--- a/routing.txt\n+++ b/routing.txt\n@@ -1 +1 @@\n-old\n+stale revision\n"
	if _, err := ApplyUnifiedPatch(PatchApplyOptions{Context: ctx, Workspace: workspace, Patch: patch}); err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("canceled patch err=%v", err)
	}
	assertPatchFile(t, target, "old\n", 0o644)
}

func assertPatchFile(t *testing.T, path, want string, mode os.FileMode) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("%s content=%q want %q", path, data, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != mode.Perm() {
		t.Fatalf("%s mode=%o want %o", path, info.Mode().Perm(), mode.Perm())
	}
}

func TestApplyUnifiedPatchUpdatesFile(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "hello.txt")
	if err := os.WriteFile(target, []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	patch := `diff --git a/hello.txt b/hello.txt
--- a/hello.txt
+++ b/hello.txt
@@ -1,3 +1,3 @@
 one
-two
+TWO
 three
`
	result, err := ApplyUnifiedPatch(PatchApplyOptions{Workspace: workspace, Patch: patch})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || result.Files[0].Action != "update" {
		t.Fatalf("unexpected result: %#v", result)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "one\nTWO\nthree\n" {
		t.Fatalf("patched file = %q", string(data))
	}
}

func TestApplyUnifiedPatchRejectsMalformedUnprefixedHunkLines(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "hello.txt")
	if err := os.WriteFile(target, []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	patch := "diff --git a/hello.txt b/hello.txt\n--- a/hello.txt\n+++ b/hello.txt\n@@ -1,2 +1,2 @@\none\n-two\n+TWO\n"
	if _, err := ApplyUnifiedPatch(PatchApplyOptions{Workspace: workspace, Patch: patch}); err == nil || !strings.Contains(err.Error(), "invalid hunk line") {
		t.Fatalf("malformed hunk err=%v, want explicit invalid-line rejection", err)
	}
	assertPatchFile(t, target, "one\ntwo\n", 0o644)
}

func TestApplyUnifiedPatchRejectsNoOpUpdate(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "hello.txt")
	if err := os.WriteFile(target, []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	patch := "diff --git a/hello.txt b/hello.txt\n--- a/hello.txt\n+++ b/hello.txt\n@@ -1,2 +1,2 @@\n one\n-two\n+two\n"
	if _, err := ApplyUnifiedPatch(PatchApplyOptions{Workspace: workspace, Patch: patch}); err == nil || !strings.Contains(err.Error(), "does not change") {
		t.Fatalf("no-op update err=%v, want explicit no-change rejection", err)
	}
	assertPatchFile(t, target, "one\ntwo\n", 0o644)
}

func TestApplyUnifiedPatchDryRunDoesNotWrite(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "hello.txt")
	if err := os.WriteFile(target, []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	patch := `diff --git a/hello.txt b/hello.txt
--- a/hello.txt
+++ b/hello.txt
@@ -1,2 +1,2 @@
 one
-two
+TWO
`
	result, err := ApplyUnifiedPatch(PatchApplyOptions{Workspace: workspace, Patch: patch, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.DryRun {
		t.Fatalf("dry run not recorded: %#v", result)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "one\ntwo\n" {
		t.Fatalf("dry run wrote file: %q", string(data))
	}
}

func TestApplyUnifiedPatchRejectsWorkspaceEscape(t *testing.T) {
	workspace := t.TempDir()
	patch := `diff --git a/../escape.txt b/../escape.txt
--- a/../escape.txt
+++ b/../escape.txt
@@ -0,0 +1 @@
+bad
`
	_, err := ApplyUnifiedPatch(PatchApplyOptions{Workspace: workspace, Patch: patch, DryRun: true})
	if err == nil {
		t.Fatal("expected workspace escape error")
	}
}

func TestApplyUnifiedPatchRejectsSymlinkWorkspaceEscape(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(workspace, "linked")); err != nil {
		t.Fatal(err)
	}
	patch := "diff --git a/linked/escape.txt b/linked/escape.txt\n--- /dev/null\n+++ b/linked/escape.txt\n@@ -0,0 +1 @@\n+escaped\n"
	_, err := ApplyUnifiedPatch(PatchApplyOptions{Workspace: workspace, Patch: patch, DryRun: true})
	if err == nil || !strings.Contains(err.Error(), "through symlink") {
		t.Fatalf("symlink escape err=%v", err)
	}
	if _, statErr := os.Stat(filepath.Join(outside, "escape.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("symlink escape wrote outside workspace: %v", statErr)
	}
}

func TestApplyUnifiedPatchCreatesFile(t *testing.T) {
	workspace := t.TempDir()
	patch := `diff --git a/new.txt b/new.txt
--- /dev/null
+++ b/new.txt
@@ -0,0 +1,2 @@
+hello
+world
`
	result, err := ApplyUnifiedPatch(PatchApplyOptions{Workspace: workspace, Patch: patch})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || result.Files[0].Action != "create" {
		t.Fatalf("unexpected result: %#v", result)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "new.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello\nworld\n" {
		t.Fatalf("created file = %q", string(data))
	}
}

func TestCreatedPatchCommitCannotReplaceTargetAppearingAfterValidation(t *testing.T) {
	workspace := t.TempDir()
	staged := filepath.Join(workspace, ".omnidex-patch-stage-race")
	target := filepath.Join(workspace, "new.txt")
	if err := os.WriteFile(staged, []byte("staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// This write represents another actor publishing the target after the
	// ordinary absence check but immediately before the atomic commit.
	if err := os.WriteFile(target, []byte("concurrent\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := commitCreatedPatchFile(staged, target); err == nil {
		t.Fatal("atomic create replaced a target that appeared after validation")
	}
	assertPatchFile(t, target, "concurrent\n", 0o600)
	assertPatchFile(t, staged, "staged\n", 0o644)
}

func TestApplyUnifiedPatchDeleteDryRunValidatesContext(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "old.txt"), []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	patch := `diff --git a/old.txt b/old.txt
--- a/old.txt
+++ /dev/null
@@ -1 +0,0 @@
-different
`
	_, err := ApplyUnifiedPatch(PatchApplyOptions{Workspace: workspace, Patch: patch, DryRun: true})
	if err == nil {
		t.Fatal("expected delete dry-run context mismatch")
	}
}

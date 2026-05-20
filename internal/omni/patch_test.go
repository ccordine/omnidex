package omni

import (
	"os"
	"path/filepath"
	"testing"
)

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

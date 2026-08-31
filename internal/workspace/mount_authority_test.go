package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAuthoritativeWorkspaceRootRename(t *testing.T) {
	workspace := t.TempDir()
	if err := os.Mkdir(filepath.Join(workspace, "destination"), 0o755); err != nil {
		t.Fatalf("create destination directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "source.txt"), []byte("accepted"), 0o640); err != nil {
		t.Fatalf("create source file: %v", err)
	}
	root := openAuthoritativeWorkspaceRootForTest(t, workspace)

	if err := root.Rename("source.txt", "destination/result.txt"); err != nil {
		t.Fatalf("rename workspace file: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(workspace, "source.txt")); !os.IsNotExist(err) {
		t.Fatalf("source still exists after rename: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(workspace, "destination", "result.txt"))
	if err != nil {
		t.Fatalf("read renamed file: %v", err)
	}
	if string(content) != "accepted" {
		t.Fatalf("renamed content = %q, want accepted", content)
	}
}

func TestAuthoritativeWorkspaceRootRenameRejectsEscapingParent(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "source.txt"), []byte("retained"), 0o600); err != nil {
		t.Fatalf("create source file: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "escape")); err != nil {
		t.Fatalf("create escaping parent symlink: %v", err)
	}
	root := openAuthoritativeWorkspaceRootForTest(t, workspace)

	if err := root.Rename("source.txt", "escape/result.txt"); err == nil {
		t.Fatal("rename through an escaping parent symlink unexpectedly succeeded")
	}
	if _, err := os.Stat(filepath.Join(workspace, "source.txt")); err != nil {
		t.Fatalf("source changed after rejected rename: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(outside, "result.txt")); !os.IsNotExist(err) {
		t.Fatalf("rename escaped the workspace root: %v", err)
	}
}

func openAuthoritativeWorkspaceRootForTest(
	t *testing.T,
	workspace string,
) *authoritativeWorkspaceRoot {
	t.Helper()
	rootFS, err := os.OpenRoot(workspace)
	if err != nil {
		t.Fatalf("open workspace root: %v", err)
	}
	directory, err := rootFS.Open(".")
	if err != nil {
		_ = rootFS.Close()
		t.Fatalf("open workspace root handle: %v", err)
	}
	mountID, err := workspaceMountIDForHandle(directory)
	if err != nil {
		_ = directory.Close()
		_ = rootFS.Close()
		t.Fatalf("resolve workspace mount: %v", err)
	}
	t.Cleanup(func() {
		if err := directory.Close(); err != nil {
			t.Errorf("close workspace root handle: %v", err)
		}
		if err := rootFS.Close(); err != nil {
			t.Errorf("close workspace root: %v", err)
		}
	})
	return &authoritativeWorkspaceRoot{
		Root: rootFS, authorityFD: int(directory.Fd()), mountID: mountID,
	}
}

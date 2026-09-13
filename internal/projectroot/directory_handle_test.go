package projectroot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirectoryHandleObservesReplacementAndClosure(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	handle, err := OpenDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	want, err := DirectoryIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := handle.Identity(); err != nil || got != want {
		t.Fatalf("opened identity = %q: %v, want %q", got, err, want)
	}
	if err := os.Rename(root, filepath.Join(parent, "retired")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := handle.Identity(); err == nil {
		t.Fatal("replaced directory retained authority")
	}
	if err := handle.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := handle.Identity(); err == nil {
		t.Fatal("closed directory retained authority")
	}
}

func TestInvokingDirectoryHandleIsAcquiredBeforeLaterSetup(t *testing.T) {
	root := filepath.Join(t.TempDir(), "invoking")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	handle, err := OpenInvokingDirectory()
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	if handle.Path() != root {
		t.Fatalf("invoking path=%q, want %q", handle.Path(), root)
	}
	if err := os.Rename(root, root+"-retained"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := handle.Identity(); err == nil {
		t.Fatal("setup retargeted a replacement directory")
	}
}

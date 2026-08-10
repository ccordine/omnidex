package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestRepositoryVerificationTreeRejectsPersistentFilesystemDrift(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "source.go"), []byte("package sample\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("source.go", filepath.Join(root, "source-link")); err != nil {
		t.Fatal(err)
	}
	before, err := captureRepositoryVerificationTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := assertRepositoryVerificationTreeUnchanged(root, before); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "unexpected.txt"), []byte("drift\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := assertRepositoryVerificationTreeUnchanged(root, before); err == nil ||
		!strings.Contains(err.Error(), "unexpected.txt") {
		t.Fatalf("inventory drift error=%v", err)
	}
	if err := os.Remove(filepath.Join(root, "unexpected.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "source.go"), []byte("package changed\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := assertRepositoryVerificationTreeUnchanged(root, before); err == nil ||
		!strings.Contains(err.Error(), "source.go") {
		t.Fatalf("content drift error=%v", err)
	}
}

func TestRepositoryVerificationTreeDoesNotFollowEscapingSymlinks(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	external := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(external, []byte("outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	before, err := captureRepositoryVerificationTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(external, []byte("changed outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := assertRepositoryVerificationTreeUnchanged(root, before); err != nil {
		t.Fatalf("external symlink target became repository authority: %v", err)
	}
	if err := os.Remove(filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("different", filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := assertRepositoryVerificationTreeUnchanged(root, before); err == nil ||
		!strings.Contains(err.Error(), "escape") {
		t.Fatalf("symlink identity drift error=%v", err)
	}
}

func TestRepositoryVerificationTreeRejectsUnsupportedEntries(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "unsupported"), 0o700); err != nil {
		t.Fatal(err)
	}
	pipe := filepath.Join(root, "unsupported", "pipe")
	if err := syscall.Mkfifo(pipe, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := captureRepositoryVerificationTree(root); err == nil ||
		!strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unsupported entry error=%v", err)
	}
}

func TestRepositoryVerificationTreeHonorsCanceledAuthority(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := captureRepositoryVerificationTreeContext(ctx, t.TempDir()); err == nil ||
		!strings.Contains(err.Error(), "authority ended") {
		t.Fatalf("canceled tree error=%v", err)
	}
}

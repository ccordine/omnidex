package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestHostDirectoryAccessAcceptsOrdinaryDirectories(t *testing.T) {
	boundary := t.TempDir()
	project := filepath.Join(boundary, "ordinary project")
	if err := os.Mkdir(project, 0o750); err != nil {
		t.Fatal(err)
	}
	access := NewHostDirectoryAccess(boundary)
	for _, root := range []string{boundary, project} {
		if err := access.ValidateWorkspaceRoot(root); err != nil {
			t.Fatalf("ordinary directory %q was rejected: %v", root, err)
		}
	}
}

func TestHostDirectoryAccessInspectsCurrentStateAtConsumer(t *testing.T) {
	boundary := filepath.Join(t.TempDir(), "created-later")
	access := NewHostDirectoryAccess(boundary)
	if err := access.ValidateWorkspaceRoot(boundary); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing directory was not reported by its consumer: %v", err)
	}
	if err := os.Mkdir(boundary, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := access.ValidateWorkspaceRoot(boundary); err != nil {
		t.Fatalf("consumer did not acquire the newly available directory: %v", err)
	}
	if err := os.Remove(boundary); err != nil {
		t.Fatal(err)
	}
	if err := access.ValidateWorkspaceRoot(boundary); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("consumer retained stale directory availability: %v", err)
	}
}

func TestHostDirectoryAccessRejectsInvalidOrEscapingDirectories(t *testing.T) {
	boundary := t.TempDir()
	outside := t.TempDir()
	file := filepath.Join(boundary, "file")
	if err := os.WriteFile(file, []byte("retained"), 0o600); err != nil {
		t.Fatal(err)
	}
	escape := filepath.Join(boundary, "escape")
	if err := os.Symlink(outside, escape); err != nil {
		t.Fatal(err)
	}
	access := NewHostDirectoryAccess(boundary)
	for _, root := range []string{"", "relative", boundary + "/../", boundary + "\x00", file, outside, escape} {
		if err := access.ValidateWorkspaceRoot(root); err == nil {
			t.Fatalf("invalid or escaping workspace %q was accepted", root)
		}
	}
	for _, configured := range []string{"", "relative", file, escape} {
		if err := NewHostDirectoryAccess(configured).ValidateWorkspaceRoot(boundary); err == nil {
			t.Fatalf("invalid configured boundary %q was accepted", configured)
		}
	}
}

func TestHostDirectoryAccessRejectsSymlinksAboveEitherRoot(t *testing.T) {
	parent := t.TempDir()
	physical := filepath.Join(parent, "physical")
	project := filepath.Join(physical, "project")
	if err := os.MkdirAll(project, 0o750); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(parent, "alias")
	if err := os.Symlink(physical, alias); err != nil {
		t.Fatal(err)
	}
	aliasedProject := filepath.Join(alias, "project")
	if err := NewHostDirectoryAccess(parent).ValidateWorkspaceRoot(aliasedProject); err == nil {
		t.Fatal("workspace with a symlink ancestor was accepted")
	}
	if err := NewHostDirectoryAccess(aliasedProject).ValidateWorkspaceRoot(aliasedProject); err == nil {
		t.Fatal("configured boundary with a symlink ancestor was accepted")
	}
}

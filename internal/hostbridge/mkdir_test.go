package hostbridge

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateDirectory(t *testing.T) {
	parent := t.TempDir()
	target, err := CreateDirectory(parent, "new-project", BrowseOptions{ExtraRoots: []string{parent}})
	if err != nil {
		t.Fatalf("CreateDirectory() error=%v", err)
	}
	stat, err := os.Stat(target)
	if err != nil || !stat.IsDir() {
		t.Fatalf("expected created directory at %q", target)
	}
}

func TestCreateDirectoryRejectsInvalidName(t *testing.T) {
	parent := t.TempDir()
	if _, err := CreateDirectory(parent, "../escape", BrowseOptions{ExtraRoots: []string{parent}}); err == nil {
		t.Fatal("expected invalid name to fail")
	}
}

func TestCreateDirectoryRejectsDuplicate(t *testing.T) {
	parent := t.TempDir()
	if err := os.MkdirAll(filepath.Join(parent, "existing"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateDirectory(parent, "existing", BrowseOptions{ExtraRoots: []string{parent}}); err == nil {
		t.Fatal("expected duplicate directory to fail")
	}
}

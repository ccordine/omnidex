package hostbridge

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWalkProjectTree(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "internal"), 0o755); err != nil {
		t.Fatalf("mkdir internal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	walk, err := WalkProjectTree(root, 100)
	if err != nil {
		t.Fatalf("WalkProjectTree() error=%v", err)
	}
	if walk.Root != root {
		t.Fatalf("root=%q want %q", walk.Root, root)
	}
	if len(walk.Files) < 2 {
		t.Fatalf("files=%d want at least 2", len(walk.Files))
	}
}

func TestWriteProjectArtifacts(t *testing.T) {
	root := t.TempDir()
	indexPath, mapPath, err := WriteProjectArtifacts(root, []byte(`{"version":"1.0"}`), []byte(`{"version":"1.0","root":"`+root+`"}`))
	if err != nil {
		t.Fatalf("WriteProjectArtifacts() error=%v", err)
	}
	if _, err := os.Stat(indexPath); err != nil {
		t.Fatalf("index file missing: %v", err)
	}
	if _, err := os.Stat(mapPath); err != nil {
		t.Fatalf("map file missing: %v", err)
	}
}

package omni

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCommandCacheKeyChangesWhenIndexedFilesChange(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "go.mod")
	if err := os.WriteFile(path, []byte("module example.com/a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := BuildWorkspaceIndex(workspace, 100)
	if err != nil {
		t.Fatal(err)
	}
	firstKey := CommandCacheKey(first, "go test ./...")
	if err := os.WriteFile(path, []byte("module example.com/b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := BuildWorkspaceIndex(workspace, 100)
	if err != nil {
		t.Fatal(err)
	}
	secondKey := CommandCacheKey(second, "go test ./...")
	if firstKey == secondKey {
		t.Fatal("cache key should change when indexed file hash changes")
	}
}

func TestSaveAndLoadCommandCacheEntry(t *testing.T) {
	root := t.TempDir()
	entry := CommandCacheEntry{
		Key:       "abc",
		Workspace: "/tmp/project",
		Command:   "go test ./...",
		InputHash: "hash",
		ExitCode:  0,
		Stdout:    "ok",
	}
	if err := SaveCommandCacheEntry(root, entry); err != nil {
		t.Fatal(err)
	}
	loaded, ok, err := LoadCommandCacheEntry(root, "abc")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected cache entry")
	}
	if loaded.Stdout != "ok" || loaded.Command != entry.Command {
		t.Fatalf("loaded entry = %#v", loaded)
	}
}

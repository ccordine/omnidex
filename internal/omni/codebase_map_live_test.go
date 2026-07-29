package omni

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodebaseMapRepresentsOnlyCurrentWorkspaceState(t *testing.T) {
	root := t.TempDir()
	for path, content := range map[string]string{
		"go.mod":          "module example.test/live\n\ngo 1.24\n",
		"cmd/app.go":      "package main\n\nfunc main() {}\n",
		"cmd/app_test.go": "package main\n",
	} {
		target := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cm, err := BuildCodebaseMap(root, CodebaseMapConfig{MaxFiles: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(cm.Files) != 3 || cm.Root != root {
		t.Fatalf("map=%+v", cm)
	}
	blob, err := json.Marshal(cm)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"\"version\"", "sha256", "stale", "summary_generated_for_hash", "last_hash", "previous_files", "reused_hashes"} {
		if strings.Contains(strings.ToLower(string(blob)), forbidden) {
			t.Errorf("live map retained version bookkeeping %q: %s", forbidden, blob)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".omni")); !os.IsNotExist(err) {
		t.Fatalf("live map wrote persistent .omni state: %v", err)
	}
}

func TestWorkspaceIndexReportsTruncation(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.go", "b.go"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("package p\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	index, err := BuildWorkspaceIndex(root, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !index.Truncated || len(index.Files) != 1 {
		t.Fatalf("index=%+v", index)
	}
}

func TestWorkspaceIndexRejectsNonDirectoryRoot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildWorkspaceIndex(path, 10); err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("non-directory workspace error=%v", err)
	}
}

func TestWorkspaceIndexRejectsMalformedPackageManifest(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"scripts":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildWorkspaceIndex(root, 10); err == nil || !strings.Contains(err.Error(), "decode package.json") {
		t.Fatalf("malformed package.json error=%v", err)
	}
}

package worker

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestTargetTreeWorkspaceSnapshotAndExistingLeafSource(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src", "ui"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "ui", "counter.tsx"), []byte("export const count = 0;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "node_modules", "ignored"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "node_modules", "ignored", "package.js"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}
	paths, directories, err := snapshotDirectCodingTargetTreePaths(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(paths, []string{"src/ui/counter.tsx"}) {
		t.Fatalf("paths=%v", paths)
	}
	if !reflect.DeepEqual(directories, []string{"src", "src/ui"}) {
		t.Fatalf("directories=%v", directories)
	}
	source, err := directCodingTargetTreeExistingSource(root, "src/ui/counter.tsx")
	if err != nil {
		t.Fatal(err)
	}
	if source != "export const count = 0;\n" {
		t.Fatalf("source=%q", source)
	}
}

package omni

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildWorkspaceIndexRecordsFilesAndManifests(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "package.json"), []byte(`{"scripts":{"test":"node test.js"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(workspace, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "src", "main.js"), []byte("console.log('ok')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".env"), []byte("SECRET=value\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	index, err := BuildWorkspaceIndex(workspace, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Files) != 2 {
		t.Fatalf("files = %d, want 2", len(index.Files))
	}
	if index.Manifests["package.json"] == "" {
		t.Fatalf("manifest hash missing: %#v", index.Manifests)
	}
	if index.PackageProbe.PackageManager != "npm" {
		t.Fatalf("package manager = %q", index.PackageProbe.PackageManager)
	}
	for _, file := range index.Files {
		if file.Path == ".env" {
			t.Fatal("workspace index should skip .env files")
		}
	}
}

func TestOmniIndexBuildCommandPrintsJSON(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module example.com/test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewApp(strings.NewReader(""), &out, &errOut)
	err := app.Run([]string{"index", "build", "--workspace", workspace, "--json"})
	if err != nil {
		t.Fatalf("index build failed: %v\nstderr=%s", err, errOut.String())
	}
	var index WorkspaceIndex
	if err := json.Unmarshal(out.Bytes(), &index); err != nil {
		t.Fatal(err)
	}
	if index.PackageProbe.PackageManager != "go" {
		t.Fatalf("index = %#v", index)
	}
}

func TestUpdateWorkspaceIndexReusesUnchangedHashes(t *testing.T) {
	workspace := t.TempDir()
	firstPath := filepath.Join(workspace, "a.txt")
	secondPath := filepath.Join(workspace, "b.txt")
	if err := os.WriteFile(firstPath, []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	initial, err := BuildWorkspaceIndex(workspace, 100)
	if err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(workspace, ".omni", "index.json")
	if err := WriteWorkspaceIndex(initial, indexPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondPath, []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	updated, err := UpdateWorkspaceIndex(workspace, indexPath, 100)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Update.ReusedHashes != 1 {
		t.Fatalf("reused hashes = %d, want 1: %#v", updated.Update.ReusedHashes, updated.Update)
	}
	if updated.Update.AddedFiles != 1 {
		t.Fatalf("added files = %d, want 1: %#v", updated.Update.AddedFiles, updated.Update)
	}
}

func TestOmniIndexUpdateCommandWritesReuseStats(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module example.com/test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewApp(strings.NewReader(""), &out, &errOut)
	if err := app.Run([]string{"index", "build", "--workspace", workspace}); err != nil {
		t.Fatalf("index build failed: %v\nstderr=%s", err, errOut.String())
	}
	out.Reset()
	if err := app.Run([]string{"index", "update", "--workspace", workspace}); err != nil {
		t.Fatalf("index update failed: %v\nstderr=%s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "reused_hashes=1") {
		t.Fatalf("index update output = %q", out.String())
	}
}

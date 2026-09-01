package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateOnlyReconciliationCreatesMissingFile(t *testing.T) {
	root := t.TempDir()
	authority := openAuthoritativeWorkspaceRootForTest(t, root)
	result := ReconciliationResult{Changes: []Change{}}
	err := applyFile(
		context.Background(), authority,
		DesiredFile{
			Path: "nested/result.txt", Present: true, Content: []byte("created"),
			Mode: 0o644, CreateOnly: true,
		},
		&result,
		nil,
	)
	if err != nil {
		t.Fatalf("apply create-only file: %v", err)
	}
	if len(result.Changes) != 1 || result.Changes[0].Kind != ChangeCreate {
		t.Fatalf("changes=%+v; want one create", result.Changes)
	}
	content, err := os.ReadFile(filepath.Join(root, "nested", "result.txt"))
	if err != nil || string(content) != "created" {
		t.Fatalf("created content=%q error=%v", content, err)
	}
}

func TestCreateOnlyReconciliationNeverReplacesExistingFile(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "result.txt")
	if err := os.WriteFile(target, []byte("user-owned"), 0o600); err != nil {
		t.Fatalf("write existing file: %v", err)
	}
	authority := openAuthoritativeWorkspaceRootForTest(t, root)
	result := ReconciliationResult{Changes: []Change{}}
	if err := applyFile(
		context.Background(), authority,
		DesiredFile{
			Path: "result.txt", Present: true, Content: []byte("replacement"),
			Mode: 0o644, CreateOnly: true,
		},
		&result,
		nil,
	); err == nil {
		t.Fatal("create-only reconciliation unexpectedly replaced an existing file")
	}
	content, err := os.ReadFile(target)
	if err != nil || string(content) != "user-owned" {
		t.Fatalf("existing content=%q error=%v", content, err)
	}
}

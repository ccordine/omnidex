package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodeOwnedWorkspaceMutationCreatesOneExactFileWithEvidence(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	result, err := applyWorkspaceFileMutation(context.Background(), root, workspaceFileMutation{
		Path: "main.go", Operation: workspaceFileCreate, Content: "package main\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(root, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "package main\n" {
		t.Fatalf("content=%q", content)
	}
	if len(result.Evidence) != 1 || result.Evidence[0].SourceRef != "main.go" {
		t.Fatalf("evidence=%#v", result.Evidence)
	}
}

func TestCodeOwnedWorkspaceMutationRejectsInvalidStateWithoutFallback(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	command := workspaceFileMutation{Path: "main.go", Operation: workspaceFileCreate, Content: "package main\n"}
	if _, err := applyWorkspaceFileMutation(context.Background(), root, command); err != nil {
		t.Fatal(err)
	}
	if _, err := applyWorkspaceFileMutation(context.Background(), root, command); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("duplicate create err=%v", err)
	}
	command.Path = "../outside.go"
	if _, err := applyWorkspaceFileMutation(context.Background(), root, command); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("escaping path err=%v", err)
	}
	command = workspaceFileMutation{Path: "other.go", Operation: "unregistered", Content: "package main\n"}
	if _, err := applyWorkspaceFileMutation(context.Background(), root, command); err == nil || !strings.Contains(err.Error(), "operation must") {
		t.Fatalf("unregistered operation err=%v", err)
	}
}

func TestCodeOwnedWorkspaceMutationRejectsOversizedExistingFileBeforeRead(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "large.go")
	file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxV3WriteBytes + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []workspaceFileOperation{workspaceFileReplace, workspaceFileDelete} {
		content := ""
		if operation == workspaceFileReplace {
			content = "package large\n"
		}
		_, err := applyWorkspaceFileMutation(context.Background(), root, workspaceFileMutation{
			Path: "large.go", Operation: operation, Content: content,
		})
		if err == nil || !strings.Contains(err.Error(), "read bound") {
			t.Fatalf("operation=%s err=%v", operation, err)
		}
	}
}

func TestNativeRuntimeRejectsRemovedConversationAction(t *testing.T) {
	t.Parallel()

	err := (&nativeRuntimeV3{action: "v3_planning"}).run()
	if err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("removed action err=%v", err)
	}
}

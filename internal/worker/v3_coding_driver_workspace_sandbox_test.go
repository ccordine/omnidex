package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

func TestDirectCodingVerificationSandboxProjectsDeltaWithoutWorkspaceCopy(t *testing.T) {
	root := t.TempDir()
	readme := strings.Repeat("source-authority-", 4096)
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte(readme), 0o644); err != nil {
		t.Fatal(err)
	}
	source, err := workspacefacts.Capture(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := workspacefacts.PlanMutation(
		t.Context(), source, "coding_"+strings.Repeat("1", 64),
		[]workspacefacts.DesiredFileState{{
			Path: "main.go", Present: true, Content: []byte("package main\n"), Mode: 0o644,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := workspacefacts.StageMutation(t.Context(), source, plan)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Cleanup() })
	projection, err := newWorkspaceStagedProjection(t.Context(), stage)
	if err != nil {
		t.Fatal(err)
	}
	sandbox, err := newDirectCodingWorkspaceSandbox(t.Context(), projection, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sandbox.Cleanup() })

	var copiedBytes int64
	err = filepath.Walk(sandbox.root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr == nil && info.Mode().IsRegular() {
			copiedBytes += info.Size()
		}
		return walkErr
	})
	if err != nil {
		t.Fatal(err)
	}
	if copiedBytes != 0 {
		t.Fatalf("verification projection copied %d source bytes", copiedBytes)
	}

	arguments, handles, infoReader, err := sandbox.invocation(context.Background(), testCommand{
		Family: "go", Name: "go", Args: []string{"test", "-count=1", "./..."},
		Purpose: verificationTest, WorkspaceRole: workspaceVerificationPrimary,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = infoReader.Close()
	for _, handle := range handles {
		_ = handle.Close()
	}
	joined := strings.Join(arguments, "\x00")
	for _, required := range []string{
		"--bind-fd\x003\x00/workspace",
		"--setenv\x00COMPOSE_PROJECT_NAME\x00omnidex",
		"/proc/self/fd/",
	} {
		if !strings.Contains(joined, required) {
			t.Fatalf("sandbox invocation lacks %q: %q", required, joined)
		}
	}
	if strings.Contains(joined, "--bind\x00"+root+"\x00/workspace") {
		t.Fatalf("sandbox invocation mounted the full workspace writable: %q", joined)
	}

	if _, err := stage.ApplyVerified(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := sandbox.VerifyAuthority(t.Context()); err != nil {
		t.Fatalf("post-apply projection authority error=%v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("drift\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := sandbox.VerifyAuthority(t.Context()); err == nil ||
		!strings.Contains(err.Error(), "README.md") {
		t.Fatalf("projection backing drift error=%v", err)
	}
}

func TestDirectCodingVerificationSandboxExecutesAgainstWritableProjection(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("retained\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	source, err := workspacefacts.Capture(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := workspacefacts.PlanMutation(
		t.Context(), source, "coding_"+strings.Repeat("2", 64),
		[]workspacefacts.DesiredFileState{
			{
				Path: "go.mod", Present: true,
				Content: []byte("module example.test/sandbox\n\ngo 1.24\n"), Mode: 0o644,
			},
			{
				Path: "main.go", Present: true,
				Content: []byte("package main\n\nfunc main() {}\n"), Mode: 0o644,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := workspacefacts.StageMutation(t.Context(), source, plan)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Cleanup() })
	projection, err := newWorkspaceStagedProjection(t.Context(), stage)
	if err != nil {
		t.Fatal(err)
	}
	sandbox, err := newDirectCodingWorkspaceSandbox(t.Context(), projection, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sandbox.Cleanup() })
	if _, err := stage.ApplyVerified(t.Context()); err != nil {
		t.Fatal(err)
	}
	result, err := sandbox.Execute(t.Context(), testCommand{
		Family: "go", Name: "go", Args: []string{"test", "-count=1", "./..."},
		Purpose: verificationTest, WorkspaceRole: workspaceVerificationPrimary,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !directCodingCommandSucceeded(result) || len(result.Evidence) != 1 {
		t.Fatalf("sandbox command result=%+v", result)
	}
	if err := plan.VerifyExpected(mustCaptureDirectCodingSandboxWorkspace(t, root)); err != nil {
		t.Fatalf("sandbox command changed authoritative workspace: %v", err)
	}
}

func mustCaptureDirectCodingSandboxWorkspace(
	t *testing.T,
	root string,
) workspacefacts.Snapshot {
	t.Helper()
	snapshot, err := workspacefacts.Capture(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

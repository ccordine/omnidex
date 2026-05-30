package hostbridge

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveHostWorkspaceUsesHostPath(t *testing.T) {
	dir := t.TempDir()
	got, err := resolveHostWorkspace(dir)
	if err != nil {
		t.Fatalf("resolveHostWorkspace: %v", err)
	}
	if got != dir {
		t.Fatalf("resolved=%q want %q", got, dir)
	}
}

func TestMapWorkspacePathForHost(t *testing.T) {
	t.Setenv("WORKSPACE_ROOT", "/workspace")
	t.Setenv("HOST_WORKSPACE_PATH", "/home/dev/projects")

	got, ok := mapWorkspacePathForHost("/workspace/omni-nxt")
	if !ok {
		t.Fatal("expected mapping")
	}
	want := filepath.Join("/home/dev/projects", "omni-nxt")
	if got != want {
		t.Fatalf("mapped=%q want %q", got, want)
	}
}

func TestResolveHostWorkspaceMapsContainerPath(t *testing.T) {
	hostRoot := t.TempDir()
	projectDir := filepath.Join(hostRoot, "repo")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("WORKSPACE_ROOT", "/workspace")
	t.Setenv("HOST_WORKSPACE_PATH", hostRoot)

	got, err := resolveHostWorkspace("/workspace/repo")
	if err != nil {
		t.Fatalf("resolveHostWorkspace: %v", err)
	}
	if got != projectDir {
		t.Fatalf("resolved=%q want %q", got, projectDir)
	}
}

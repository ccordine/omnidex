package api

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCollectProjectGitStatusNonRepo(t *testing.T) {
	dir := t.TempDir()
	payload, err := collectProjectGitStatus(context.Background(), dir)
	if err != nil {
		t.Fatalf("collectProjectGitStatus: %v", err)
	}
	if payload["is_repo"] != false {
		t.Fatalf("expected is_repo=false, got %#v", payload["is_repo"])
	}
}

func TestCollectProjectGitStatusRepo(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "Initial commit")

	payload, err := collectProjectGitStatus(context.Background(), dir)
	if err != nil {
		t.Fatalf("collectProjectGitStatus: %v", err)
	}
	if payload["is_repo"] != true {
		t.Fatalf("expected is_repo=true, got %#v", payload["is_repo"])
	}
	if payload["clean"] != true {
		t.Fatalf("expected clean repo, got %#v", payload)
	}
	commits, ok := payload["recent_commits"].([]map[string]any)
	if !ok || len(commits) == 0 {
		t.Fatalf("expected recent commits, got %#v", payload["recent_commits"])
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

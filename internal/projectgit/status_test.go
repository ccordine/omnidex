package projectgit

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCollectStatusNonRepo(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	payload, err := CollectStatus(context.Background(), dir, "core-local")
	if err != nil {
		t.Fatalf("CollectStatus: %v", err)
	}
	if payload.IsRepo {
		t.Fatalf("expected is_repo=false, got %#v", payload.IsRepo)
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

func TestCollectStatusRepo(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "Initial commit")

	payload, err := CollectStatus(context.Background(), dir, "core-local")
	if err != nil {
		t.Fatalf("CollectStatus: %v", err)
	}
	if !payload.IsRepo {
		t.Fatalf("expected is_repo=true, got %#v", payload.IsRepo)
	}
}

func TestCollectStatusLinkedWorktree(t *testing.T) {
	repository := t.TempDir()
	runGit(t, repository, "init")
	runGit(t, repository, "config", "user.email", "test@example.com")
	runGit(t, repository, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repository, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "add", "README.md")
	runGit(t, repository, "commit", "-m", "Initial commit")
	worktree := filepath.Join(t.TempDir(), "linked")
	runGit(t, repository, "worktree", "add", "-b", "linked-branch", worktree)

	payload, err := CollectStatus(context.Background(), worktree, "core-local")
	if err != nil {
		t.Fatalf("CollectStatus linked worktree: %v", err)
	}
	if !payload.IsRepo || payload.Branch != "linked-branch" {
		t.Fatalf("linked worktree status=%+v", payload)
	}
}

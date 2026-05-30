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
	payload, err := CollectStatus(context.Background(), dir, "test")
	if err != nil {
		t.Fatalf("CollectStatus: %v", err)
	}
	if payload["is_repo"] != false {
		t.Fatalf("expected is_repo=false, got %#v", payload["is_repo"])
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

	payload, err := CollectStatus(context.Background(), dir, "test")
	if err != nil {
		t.Fatalf("CollectStatus: %v", err)
	}
	if payload["is_repo"] != true {
		t.Fatalf("expected is_repo=true, got %#v", payload["is_repo"])
	}
}

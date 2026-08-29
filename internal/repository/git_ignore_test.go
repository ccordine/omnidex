package repository

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequireGitPathVisibleRejectsIgnoredTarget(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
	} {
		command := exec.Command("git", args...)
		command.Dir = root
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RequireGitPathVisible(t.Context(), root, "visible.txt"); err != nil {
		t.Fatalf("visible target: %v", err)
	}
	if err := RequireGitPathVisible(t.Context(), root, "ignored.txt"); err == nil ||
		!strings.Contains(err.Error(), "ignored") {
		t.Fatalf("ignored target error=%v", err)
	}
}

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectgit"
)

func TestCollectProjectGitStatusNonRepo(t *testing.T) {
	dir := t.TempDir()
	payload, err := projectgit.CollectStatus(context.Background(), dir, "core-local")
	if err != nil {
		t.Fatalf("CollectStatus: %v", err)
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

	payload, err := projectgit.CollectStatus(context.Background(), dir, "core-local")
	if err != nil {
		t.Fatalf("CollectStatus: %v", err)
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

func TestLoadProjectGitStatusUsesHostBridgeWhenCoreMissing(t *testing.T) {
	coreDir := t.TempDir()
	missingPath := filepath.Join(coreDir, "bridge-only")

	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/project/git" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("path") != missingPath {
			t.Fatalf("git path=%q want %q", r.URL.Query().Get("path"), missingPath)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"location": missingPath,
			"source":   "host-bridge",
			"is_repo":  true,
			"clean":    true,
			"branch":   "main",
		})
	}))
	t.Cleanup(host.Close)

	server := NewServerWithOptions(nil, &fakeLLMClient{}, ServerOptions{HostAgentURL: host.URL})
	payload, err := server.loadProjectGitStatus(context.Background(), model.Project{}, missingPath)
	if err != nil {
		t.Fatalf("loadProjectGitStatus: %v", err)
	}
	if payload["is_repo"] != true {
		t.Fatalf("expected is_repo=true, got %#v", payload)
	}
	if payload["source"] != "host-bridge" {
		t.Fatalf("source=%#v want host-bridge", payload["source"])
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

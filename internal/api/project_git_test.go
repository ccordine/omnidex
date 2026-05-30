package api

import (
	"context"
	"encoding/json"
	"errors"
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
	hostDir := t.TempDir()
	projectDir := filepath.Join(hostDir, "repo")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/browse":
			path := r.URL.Query().Get("path")
			if path != projectDir {
				t.Fatalf("browse path=%q want %q", path, projectDir)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"path":    projectDir,
				"parent":  hostDir,
				"entries": []any{},
			})
		case "/v1/project/git":
			if r.URL.Query().Get("path") != projectDir {
				t.Fatalf("git path=%q want %q", r.URL.Query().Get("path"), projectDir)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"location": projectDir,
				"source":   "host-bridge",
				"is_repo":  true,
				"clean":    true,
				"branch":   "main",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(host.Close)

	t.Setenv("WORKSPACE_ROOT", "/workspace")
	t.Setenv("HOST_WORKSPACE_PATH", hostDir)

	server := NewServerWithOptions(nil, &fakeLLMClient{}, ServerOptions{HostAgentURL: host.URL})
	payload, err := server.loadProjectGitStatus(context.Background(), model.Project{}, "/workspace/repo")
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

func TestProjectGitBridgeErrorForMissingRoute(t *testing.T) {
	err := projectGitBridgeError(errors.New("host bridge HTTP 404: 404 page not found"))
	if err == nil {
		t.Fatal("expected error")
	}
	if got, want := err.Error(), "host bridge does not expose project git status yet; restart or update omni-host-bridge"; got != want {
		t.Fatalf("error=%q want %q", got, want)
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

package hostbridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestHandleProjectGitRepo(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "Initial commit")

	server := &Server{}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	resp, err := http.Get(httpServer.URL + "/v1/project/git?path=" + filepath.Clean(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload["is_repo"] != true {
		t.Fatalf("expected is_repo=true, got %#v", payload)
	}
	if payload["source"] != "host-bridge" {
		t.Fatalf("source=%#v want host-bridge", payload["source"])
	}
}

func TestClientProjectGitStatus(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")

	server := &Server{}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	client := NewClient(httpServer.URL, "", 0)
	payload, err := client.ProjectGitStatus(context.Background(), dir)
	if err != nil {
		t.Fatalf("ProjectGitStatus: %v", err)
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

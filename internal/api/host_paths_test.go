package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/hostbridge"
)

func TestMapWorkspacePathForHostBridge(t *testing.T) {
	t.Setenv("WORKSPACE_ROOT", "/workspace")
	t.Setenv("HOST_WORKSPACE_PATH", "/home/dev/projects")

	got, ok := mapWorkspacePathForHostBridge("/workspace/omni-nxt")
	if !ok {
		t.Fatal("expected mapping")
	}
	want := filepath.Join("/home/dev/projects", "omni-nxt")
	if got != want {
		t.Fatalf("mapped=%q want=%q", got, want)
	}
}

func TestResolveHostBridgeProjectPathUsesWorkspaceMapping(t *testing.T) {
	hostDir := t.TempDir()
	projectDir := filepath.Join(hostDir, "repo")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/browse" {
			http.NotFound(w, r)
			return
		}
		path := r.URL.Query().Get("path")
		if path != projectDir {
			t.Fatalf("browse path=%q want %q", path, projectDir)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"path":    projectDir,
			"parent":  hostDir,
			"entries": []any{},
		})
	}))
	t.Cleanup(host.Close)

	t.Setenv("WORKSPACE_ROOT", "/workspace")
	t.Setenv("HOST_WORKSPACE_PATH", hostDir)

	client := hostbridge.NewClient(host.URL, "", 0)
	got, err := resolveHostBridgeProjectPath(context.Background(), client, "/workspace/repo")
	if err != nil {
		t.Fatalf("resolveHostBridgeProjectPath: %v", err)
	}
	if got != projectDir {
		t.Fatalf("resolved=%q want %q", got, projectDir)
	}
}

func TestTerminalBridgeDialErrorBadHandshake(t *testing.T) {
	message := terminalBridgeDialError(errors.New("websocket: bad handshake"), nil)
	if !strings.Contains(message, "handshake failed") {
		t.Fatalf("message=%q", message)
	}
}

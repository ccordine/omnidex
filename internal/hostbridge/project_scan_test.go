package hostbridge

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestWalkProjectTree(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "internal"), 0o755); err != nil {
		t.Fatalf("mkdir internal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	walk, err := WalkProjectTree(root, 100)
	if err != nil {
		t.Fatalf("WalkProjectTree() error=%v", err)
	}
	if walk.Root != root {
		t.Fatalf("root=%q want %q", walk.Root, root)
	}
	if len(walk.Files) < 2 {
		t.Fatalf("files=%d want at least 2", len(walk.Files))
	}
}

func TestProjectMapPersistenceEndpointsAreRemoved(t *testing.T) {
	server := &Server{}
	get := httptest.NewRequest(http.MethodGet, "/v1/project-map", nil)
	getResult := httptest.NewRecorder()
	server.Handler().ServeHTTP(getResult, get)
	if getResult.Code != http.StatusNotFound {
		t.Fatalf("old project-map read status=%d", getResult.Code)
	}

	post := httptest.NewRequest(http.MethodPost, "/v1/project-map/scan", bytes.NewBufferString(`{"path":"/tmp","index_json":{},"map_json":{}}`))
	post.Header.Set("Content-Type", "application/json")
	postResult := httptest.NewRecorder()
	server.Handler().ServeHTTP(postResult, post)
	if postResult.Code != http.StatusBadRequest {
		t.Fatalf("old persistence payload status=%d body=%s", postResult.Code, postResult.Body.String())
	}
}

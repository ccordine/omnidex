package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestProjectsRequireDatabase(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	req := httptest.NewRequest(http.MethodGet, "/v1/projects", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want %d body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
}

func TestBrowseDefaultsToHome(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	req := httptest.NewRequest(http.MethodGet, "/v1/browse", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRecipesList(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	req := httptest.NewRequest(http.MethodGet, "/v1/recipes", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSplitProjectPath(t *testing.T) {
	id, action := splitProjectPath("/v1/projects/42/activate")
	if id != 42 || action != "activate" {
		t.Fatalf("id=%d action=%q", id, action)
	}
	id, action = splitProjectPath("/v1/projects/7/map/scan")
	if id != 7 || action != "map/scan" {
		t.Fatalf("id=%d action=%q", id, action)
	}
}

func TestValidateProjectLocationUsesHostBridgeWhenCoreMissing(t *testing.T) {
	hostDir := t.TempDir()
	hostPath := filepath.Join(hostDir, "existing")
	if err := os.MkdirAll(hostPath, 0o755); err != nil {
		t.Fatal(err)
	}

	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/browse" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("path") != hostPath {
			t.Fatalf("browse path=%q want %q", r.URL.Query().Get("path"), hostPath)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"path":    hostPath,
			"parent":  hostDir,
			"entries": []any{},
		})
	}))
	t.Cleanup(host.Close)

	server := NewServerWithOptions(nil, &fakeLLMClient{}, ServerOptions{HostAgentURL: host.URL})
	got, err := server.validateProjectLocation(context.Background(), hostPath)
	if err != nil {
		t.Fatalf("validateProjectLocation: %v", err)
	}
	if got != hostPath {
		t.Fatalf("location=%q want %q", got, hostPath)
	}
}

func TestValidateProjectLocationPrefersCoreFilesystem(t *testing.T) {
	localDir := t.TempDir()
	server := NewServer(nil, &fakeLLMClient{})
	got, err := server.validateProjectLocation(context.Background(), localDir)
	if err != nil {
		t.Fatalf("validateProjectLocation: %v", err)
	}
	if got != localDir {
		t.Fatalf("location=%q want %q", got, localDir)
	}
}

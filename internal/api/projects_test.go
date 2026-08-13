package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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
	var payload struct {
		Limit   int   `json:"limit"`
		Offset  int   `json:"offset"`
		HasMore bool  `json:"has_more"`
		Entries []any `json:"entries"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Limit != 50 || payload.Offset != 0 || payload.Entries == nil {
		t.Fatalf("unexpected browse page: %+v", payload)
	}
}

func TestBrowseReturnsBoundedPageMetadata(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	for _, name := range []string{"one", "two", "three"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	server := NewServer(nil, &fakeLLMClient{})
	req := httptest.NewRequest(http.MethodGet, "/v1/browse?path="+url.QueryEscape(root)+"&limit=2&offset=0", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Limit          int   `json:"limit"`
		Offset         int   `json:"offset"`
		HasPrevious    bool  `json:"has_previous"`
		PreviousOffset int   `json:"previous_offset"`
		HasMore        bool  `json:"has_more"`
		NextOffset     int   `json:"next_offset"`
		Entries        []any `json:"entries"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Limit != 2 || payload.Offset != 0 || !payload.HasMore || payload.NextOffset != 2 || len(payload.Entries) != 2 {
		t.Fatalf("unexpected browse page: %+v", payload)
	}
	secondReq := httptest.NewRequest(http.MethodGet, "/v1/browse?path="+url.QueryEscape(root)+"&limit=2&offset=2", nil)
	secondRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusOK {
		t.Fatalf("second status=%d body=%s", secondRec.Code, secondRec.Body.String())
	}
	if err := json.NewDecoder(secondRec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if !payload.HasPrevious || payload.PreviousOffset != 0 || payload.HasMore || len(payload.Entries) != 1 {
		t.Fatalf("unexpected second browse page: %+v", payload)
	}
}

func TestProjectBrowseModalRendersServerPaginationControls(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	for index := 0; index < 27; index++ {
		if err := os.Mkdir(filepath.Join(root, fmt.Sprintf("directory-%02d", index)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	server := NewServer(nil, &fakeLLMClient{})
	request := func(offset int) string {
		raw := fmt.Sprintf("/v1/ui/projects/modal?kind=browse&mode=create&path=%s&offset=%d", url.QueryEscape(root), offset)
		rec := httptest.NewRecorder()
		server.handleUIProjectModal(rec, httptest.NewRequest(http.MethodGet, raw, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("offset=%d status=%d body=%s", offset, rec.Code, rec.Body.String())
		}
		var payload struct {
			HTML struct {
				Bundle string `json:"bundle"`
			} `json:"html"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		return payload.HTML.Bundle
	}
	if first := request(0); !strings.Contains(first, ">Next</button>") || strings.Contains(first, ">Previous</button>") {
		t.Fatalf("first page controls are invalid: %s", first)
	}
	if second := request(25); !strings.Contains(second, ">Previous</button>") || strings.Contains(second, ">Next</button>") {
		t.Fatalf("last page controls are invalid: %s", second)
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
	id, action := splitProjectPath("/v1/projects/42/play")
	if id != 42 || action != "play" {
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
			"path": hostPath, "parent": hostDir, "entries": []any{},
			"limit": 1, "offset": 0, "has_previous": false, "previous_offset": 0,
			"has_more": false, "next_offset": 0,
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

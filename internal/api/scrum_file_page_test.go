package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestExactScrumProjectBrowsePathRejectsEscape(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "pkg")
	got, err := exactScrumProjectBrowsePath(root, inside)
	if err != nil || got != inside {
		t.Fatalf("inside path=%q error=%v", got, err)
	}
	if _, err := exactScrumProjectBrowsePath(root, filepath.Join(root, "..", "escape")); err == nil {
		t.Fatal("project file browse escape was accepted")
	}
}

func TestScrumFilePageUsesLocalProjectWhenBridgeIsConfigured(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "local.go"), []byte("package local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := &Server{repo: &queue.Repository{}, hostAgentURL: "http://127.0.0.1:1"}
	request := httptest.NewRequest(http.MethodGet, "/v1/scrum/cards/card/modal", nil)
	page, err := server.scrumProjectFilePage(request, root, root, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) != 1 || page.Entries[0].Name != "local.go" || page.RequiredRoot == "" {
		t.Fatalf("local page=%+v", page)
	}
}

func TestScrumFilePageMapsHostOnlyProjectAndAttestsRoot(t *testing.T) {
	hostRoot := t.TempDir()
	hostProject := filepath.Join(hostRoot, "repo")
	if err := os.MkdirAll(hostProject, 0o755); err != nil {
		t.Fatal(err)
	}
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("path"); got != hostProject {
			t.Fatalf("host path=%q want %q", got, hostProject)
		}
		requiredRoot := r.URL.Query().Get("required_root")
		entries := []any{}
		if requiredRoot != "" {
			if requiredRoot != hostProject {
				t.Fatalf("required root=%q want %q", requiredRoot, hostProject)
			}
			entries = []any{map[string]any{
				"name": "host.go", "path": filepath.Join(hostProject, "host.go"), "is_dir": false,
			}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"path": hostProject, "parent": hostRoot, "entries": entries,
			"limit":  map[bool]int{true: scrumFilePageSize, false: 1}[requiredRoot != ""],
			"offset": 0, "has_previous": false, "previous_offset": 0,
			"has_more": false, "next_offset": 0, "required_root": requiredRoot,
		})
	}))
	t.Cleanup(bridge.Close)
	t.Setenv("WORKSPACE_ROOT", "/workspace")
	t.Setenv("HOST_WORKSPACE_PATH", hostRoot)
	server := &Server{repo: &queue.Repository{}, hostAgentURL: bridge.URL}
	request := httptest.NewRequest(http.MethodGet, "/v1/scrum/cards/card/modal", nil)
	ctx := &scrumModalRenderContext{}
	if err := server.populateScrumModalFileContext(request, "/workspace/repo", "", 0, ctx); err != nil {
		t.Fatal(err)
	}
	if len(ctx.Files) != 1 || ctx.Files[0] != "host.go" || ctx.FilePath != "" || ctx.FileHasParent {
		t.Fatalf("host-mapped modal page=%+v", ctx)
	}
}

func TestScrumFilePageSupportsSymlinkProjectRootButRejectsNestedEscape(t *testing.T) {
	realRoot := t.TempDir()
	logicalParent := t.TempDir()
	logicalRoot := filepath.Join(logicalParent, "repo")
	if err := os.Symlink(realRoot, logicalRoot); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realRoot, "inside.go"), []byte("package inside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(realRoot, "escape")); err != nil {
		t.Fatal(err)
	}
	server := &Server{repo: &queue.Repository{}}
	request := httptest.NewRequest(http.MethodGet, "/v1/scrum/cards/card/modal", nil)
	ctx := &scrumModalRenderContext{}
	if err := server.populateScrumModalFileContext(request, logicalRoot, "", 0, ctx); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, path := range ctx.Files {
		seen[path] = true
	}
	if len(ctx.Files) != 2 || !seen["escape"] || !seen["inside.go"] {
		t.Fatalf("symlink-root page=%+v", ctx)
	}
	escapeRequest := httptest.NewRequest(http.MethodGet, "/v1/scrum/cards/card/modal", nil)
	if err := server.populateScrumModalFileContext(
		escapeRequest, logicalRoot, "escape", 0, &scrumModalRenderContext{},
	); err == nil {
		t.Fatal("nested symlink escape was accepted")
	}
}

func TestScrumProjectRelativePathIsRootBound(t *testing.T) {
	root := t.TempDir()
	got, err := scrumProjectRelativePath(root, filepath.Join(root, "pkg", "a.go"))
	if err != nil || got != "pkg/a.go" {
		t.Fatalf("relative path=%q error=%v", got, err)
	}
	if _, err := scrumProjectRelativePath(root, filepath.Join(root, "..", "escape")); err == nil {
		t.Fatal("relative file projection escaped project root")
	}
}

package api

import (
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/omni"
)

func TestCodebaseMapPayloadSummarizesFiles(t *testing.T) {
	cm := omni.CodebaseMap{
		Root:        "/tmp/project",
		GeneratedAt: "2026-05-29T12:00:00Z",
		Revision:    "abc123",
		Files: []omni.FileSummary{
			{Path: "internal/api/server.go", Language: "go", Module: "internal/api", Purpose: "HTTP server"},
			{Path: "README.md", Language: "markdown", Module: ".", Purpose: "docs", Stale: true},
		},
		Modules: []omni.ModuleSummary{
			{Path: "internal/api", Purpose: "API layer", ImportantFiles: []string{"internal/api/server.go"}},
		},
		Languages: []omni.LanguageSummary{{Language: "go", Files: 1}},
	}
	payload := codebaseMapPayload(cm, "/tmp/project/.omni/codebase-map.json", true)
	if payload["file_count"] != 2 {
		t.Fatalf("file_count=%#v", payload["file_count"])
	}
	if payload["stale_file_count"] != 1 {
		t.Fatalf("stale_file_count=%#v", payload["stale_file_count"])
	}
	preview, ok := payload["files_preview"].([]map[string]any)
	if !ok || len(preview) != 2 {
		t.Fatalf("files_preview=%#v", payload["files_preview"])
	}
}

func TestCodebaseMapPayloadEmpty(t *testing.T) {
	payload := codebaseMapPayload(omni.CodebaseMap{}, "/tmp/.omni/codebase-map.json", false)
	if payload["exists"] != false {
		t.Fatalf("expected exists=false, got %#v", payload["exists"])
	}
	if payload["file_count"] != 0 {
		t.Fatalf("expected file_count=0, got %#v", payload["file_count"])
	}
}

func TestProjectPathAccessibleLocally(t *testing.T) {
	dir := t.TempDir()
	if !projectPathAccessibleLocally(dir) {
		t.Fatalf("expected temp dir to be accessible locally")
	}
	if projectPathAccessibleLocally(filepath.Join(dir, "missing")) {
		t.Fatalf("expected missing path to be inaccessible")
	}
}

package api

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/projectgit"
)

func TestRenderUIProjectInspectionRequiresExactTypedPayload(t *testing.T) {
	mapPayload := map[string]any{"tree_preview": "a.go\nb.go", "file_count": 2, "module_count": int64(1)}
	html, err := renderUIProjectMap(7, mapPayload)
	if err != nil || !strings.Contains(html, "a.go") || !strings.Contains(html, ">2<") {
		t.Fatalf("render map html=%q err=%v", html, err)
	}
	gitPayload := validUIProjectGitStatus()
	gitPayload.StagedCount, gitPayload.ModifiedCount, gitPayload.UntrackedCount, gitPayload.DeletedCount = 1, 2, 3, 4
	gitPayload.Clean = false
	html, err = renderUIProjectGit(7, gitPayload)
	if err != nil || !strings.Contains(html, "main") || !strings.Contains(html, "abc123") {
		t.Fatalf("render git html=%q err=%v", html, err)
	}
}

func TestRenderUIProjectInspectionRejectsMissingAndWrongTypedState(t *testing.T) {
	mapCases := []map[string]any{
		{"tree_preview": "a.go", "module_count": 1},
		{"tree_preview": "a.go", "file_count": "2", "module_count": 1},
		{"tree_preview": "a.go", "file_count": -1, "module_count": 1},
	}
	for _, payload := range mapCases {
		if html, err := renderUIProjectMap(7, payload); err == nil || html != "" {
			t.Fatalf("map payload %#v rendered fallback html=%q err=%v", payload, html, err)
		}
	}
	gitCases := []projectgit.Status{{}, validUIProjectGitStatus()}
	gitCases[1].LastCommit = nil
	for _, payload := range gitCases {
		if html, err := renderUIProjectGit(7, payload); err == nil || html != "" {
			t.Fatalf("git payload %#v rendered fallback html=%q err=%v", payload, html, err)
		}
	}
}

func validUIProjectGitStatus() projectgit.Status {
	commit := projectgit.Commit{Hash: "abc123abc123", Subject: "Initial", Author: "Test", RelativeDate: "now"}
	return projectgit.Status{
		Location: "/srv/project", Source: "core-local", IsRepo: true,
		Root: "/srv/project", Branch: "main", HeadShort: "abc123",
		ChangedFiles: []projectgit.ChangedFile{}, Clean: true,
		RecentCommits: []projectgit.Commit{commit}, LastCommit: &commit,
	}
}

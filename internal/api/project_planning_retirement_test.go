package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectPlanningParallelAuthorityIsAbsentFromProduction(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"project_planning_drafts.go", "project_planning_http.go",
		filepath.Join("..", "queue", "project_planning.go"),
		filepath.Join("..", "queue", "project_planning_store.go"),
		filepath.Join("..", "queue", "project_planning_draft_actions.go"),
		filepath.Join("..", "model", "project_planning.go"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("retired project-planning authority still exists at %s: %v", path, err)
		}
	}
	raw, err := os.ReadFile("project_planning_chat.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"ListProjectPlanning", "web_search_registered", "projectPlanningStatePayload",
	} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("retired project-planning source retains %q", forbidden)
		}
	}
}

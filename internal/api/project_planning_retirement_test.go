package api

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectPlanningParallelAuthorityIsAbsentFromProduction(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"project_planning_chat.go", "project_planning_drafts.go", "project_planning_http.go",
		filepath.Join("..", "queue", "project_planning.go"),
		filepath.Join("..", "queue", "project_planning_store.go"),
		filepath.Join("..", "queue", "project_planning_draft_actions.go"),
		filepath.Join("..", "model", "project_planning.go"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("retired project-planning authority still exists at %s: %v", path, err)
		}
	}
}

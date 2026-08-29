package queue

import (
	"os"
	"strings"
	"testing"
)

func TestProjectPlanningRetirementMigrationFailsClosedBeforeDroppingState(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/086_project_planning_retirement.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"retained_rows <> 0", "RAISE EXCEPTION", "export or explicitly discard",
		"model <> ''", "reasoning_mode <> 'instant'", "non-default configuration rows",
		"DROP TABLE project_planning_messages", "DROP TABLE project_planning_drafts",
		"DROP TABLE project_planning_configs", "to_regclass",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("project planning retirement migration is missing %q", required)
		}
	}
	for _, forbidden := range []string{"TRUNCATE", "DELETE FROM", "CASCADE"} {
		if strings.Contains(source, forbidden) {
			t.Errorf("project planning retirement silently destroys state through %q", forbidden)
		}
	}
}

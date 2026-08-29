package queue

import (
	"os"
	"strings"
	"testing"
)

func TestRetiredPipelineMigrationFailsClosedAndInstallsDatabaseAuthority(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/087_executable_pipeline_authority.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"LOCK TABLE jobs IN ACCESS EXCLUSIVE MODE",
		"pipeline NOT IN ('chat','coding','scrum')",
		"status NOT IN ('completed','failed','canceled')",
		"RAISE EXCEPTION",
		"jobs_executable_pipeline_authority",
		"enforce_jobs_executable_pipeline_authority",
		"TG_OP='INSERT'",
		"NEW IS DISTINCT FROM OLD",
		"historical retired job is immutable",
		"current job pipeline cannot become retired or unregistered",
		"jobs_history_truncate_immutable",
		"job history is immutable",
		"OLD.status IN ('completed','failed','canceled')",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("pipeline authority migration is missing %q", required)
		}
	}
	for _, forbidden := range []string{"DELETE FROM jobs", "UPDATE jobs SET pipeline", "assistant'", "story'"} {
		if strings.Contains(source, forbidden) {
			t.Errorf("pipeline authority migration contains forbidden compatibility mutation %q", forbidden)
		}
	}
}

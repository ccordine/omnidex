package queue

import (
	"os"
	"strings"
	"testing"
)

const retiredExecutionAuthorityMigration = "091_retired_execution_authority.sql"

func TestRetiredExecutionAuthorityMigrationIsCleanStartOnly(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + retiredExecutionAuthorityMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"migration 091 reset required",
		"LOCK TABLE projects, scrum_cards, workspace_settings, jobs, job_steps, omni_runs IN ACCESS EXCLUSIVE MODE",
		"DROP COLUMN recipe_id", "DROP COLUMN recipe",
		"DROP COLUMN agent_config", "DROP COLUMN model_config", "DROP COLUMN coach_config",
		"DROP COLUMN tags_job_id", "DROP COLUMN ticket_job_id",
		"DROP COLUMN external_agents_used",
		"DROP FUNCTION omni_valid_agent_config(JSONB, BOOLEAN)",
		"jobs_retired_execution_metadata_absent",
		"job_steps_retired_external_action_absent",
		"projects_retired_agent_config_absent",
		"workspace_settings_retired_agent_config_absent",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("migration lacks %q", required)
		}
	}
	for _, key := range []string{
		"agent_config", "agent_config_source", "instance_agent_config",
		"external_agents_used", "execution_agent", "agent_strict",
		"scrum_raw_play", "omnidex_no_delegate", "recipe_id", "recipe",
	} {
		if !strings.Contains(source, "'"+key+"'") {
			t.Errorf("migration lacks retired job metadata key %q", key)
		}
	}
	for _, forbidden := range []string{
		"CREATE TABLE", "CREATE TEMP", "archive", "witness", "compatibility",
		"UPDATE projects", "UPDATE scrum_cards", "UPDATE jobs", "DELETE FROM",
	} {
		if strings.Contains(strings.ToLower(source), strings.ToLower(forbidden)) {
			t.Errorf("clean-start migration contains forbidden preservation or repair token %q", forbidden)
		}
	}
}

package queue

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresRetiredExecutionAuthorityCleanCutoverAndFutureGuards(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "090")); err != nil {
		t.Fatal(err)
	}
	project, err := repository.CreateProject(t.Context(), "Current project", "/srv/current-project", "")
	if err != nil {
		t.Fatal(err)
	}
	card, err := repository.CreateScrumCard(
		t.Context(), project.ID, "current-card", "Current card", "", "assigned",
		json.RawMessage(`[]`), json.RawMessage(`[]`),
	)
	if err != nil {
		t.Fatal(err)
	}
	fixture := seedPreInlineExecutionMigrationJob(
		t, t.Context(), pool, "current coding work",
		model.PipelineCoding, "v3_coding", nil,
	)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "091")); err != nil {
		t.Fatal(err)
	}
	var stepCount int
	if err := pool.QueryRow(t.Context(), `SELECT COUNT(*) FROM job_steps WHERE job_id=$1`, fixture.Job.ID).Scan(&stepCount); err != nil {
		t.Fatal(err)
	}
	if stepCount != 1 {
		t.Fatalf("current coding job step count=%d want 1", stepCount)
	}

	var retiredColumnCount, retiredFunctionCount int
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema=current_schema() AND (
		  (table_name='projects' AND column_name IN ('recipe_id','recipe')) OR
		  (table_name='scrum_cards' AND column_name IN
		    ('agent_config','model_config','coach_config','recipe_id','recipe','tags_job_id','ticket_job_id')) OR
		  (table_name='omni_runs' AND column_name IN ('recipe_id','external_agents_used')))
	`).Scan(&retiredColumnCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM pg_proc AS p JOIN pg_namespace AS n ON n.oid=p.pronamespace
		WHERE n.nspname=current_schema() AND p.proname='omni_valid_agent_config'
	`).Scan(&retiredFunctionCount); err != nil {
		t.Fatal(err)
	}
	if retiredColumnCount != 0 || retiredFunctionCount != 0 {
		t.Fatalf("retired catalog columns/functions=%d/%d", retiredColumnCount, retiredFunctionCount)
	}
	var currentCardCount int
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM scrum_cards WHERE project_id=$1 AND id=$2 AND title='Current card'
	`, project.ID, card.ID).Scan(&currentCardCount); err != nil {
		t.Fatal(err)
	}
	if currentCardCount != 1 {
		t.Fatalf("current card count=%d want 1", currentCardCount)
	}
	currentCard, err := repository.GetScrumCard(t.Context(), project.ID, card.ID)
	if err != nil {
		t.Fatalf("read current card after retired-column drop: %v", err)
	}
	if currentCard.ID != card.ID || currentCard.Title != card.Title {
		t.Fatalf("current card changed across retired-column drop: %#v", currentCard)
	}

	for _, key := range []string{
		"agent_config", "agent_config_source", "instance_agent_config",
		"external_agents_used", "execution_agent", "agent_strict",
		"scrum_raw_play", "omnidex_no_delegate", "recipe_id", "recipe",
	} {
		_, err := pool.Exec(t.Context(), `UPDATE jobs SET metadata=jsonb_build_object($2::text,true) WHERE id=$1`, fixture.Job.ID, key)
		if err == nil || !strings.Contains(err.Error(), "jobs_retired_execution_metadata_absent") {
			t.Errorf("retired metadata key %q update error=%v", key, err)
		}
	}
	if tag, err := pool.Exec(t.Context(), `
		UPDATE jobs SET metadata=jsonb_build_object('channel_id','channel-one','channel_user_message_id',1)
		WHERE id=$1
	`, fixture.Job.ID); err != nil {
		t.Fatalf("legitimate internal channel binding rejected: %v", err)
	} else if tag.RowsAffected() != 1 {
		t.Fatalf("legitimate internal channel binding updated %d rows", tag.RowsAffected())
	}
	if tag, err := pool.Exec(t.Context(), `
		UPDATE job_steps SET action='external_agent_execute' WHERE job_id=$1
	`, fixture.Job.ID); err == nil || !strings.Contains(err.Error(), "job_steps_retired_external_action_absent") {
		t.Fatalf("retired job step update rows=%d error=%v", tag.RowsAffected(), err)
	}
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO job_steps(job_id,action,sort_index,status,generation)
		VALUES($1,'external_agent_execute',99,'pending',1)
	`, fixture.Job.ID); err == nil || !strings.Contains(err.Error(), "job_steps_retired_external_action_absent") {
		t.Fatalf("retired job step insert error=%v", err)
	}
	if _, err := pool.Exec(t.Context(), `UPDATE projects SET settings='{"agent_config":{}}'::jsonb WHERE id=$1`, project.ID); err == nil ||
		!strings.Contains(err.Error(), "projects_retired_agent_config_absent") {
		t.Fatalf("retired project setting update error=%v", err)
	}
	if _, err := pool.Exec(t.Context(), `UPDATE projects SET settings='{"model_config":{}}'::jsonb WHERE id=$1`, project.ID); err != nil {
		t.Fatalf("current project model routing rejected: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `INSERT INTO workspace_settings(key,value) VALUES('workspace_agent_config','{}')`); err == nil ||
		!strings.Contains(err.Error(), "workspace_settings_retired_agent_config_absent") {
		t.Fatalf("retired workspace setting insert error=%v", err)
	}
	if _, err := pool.Exec(t.Context(), `INSERT INTO workspace_settings(key,value) VALUES('api_secrets','{}')`); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{`{"cursor_api_key":"retired"}`, `{"codex_api_key":"retired"}`, `null`, `[]`} {
		_, err := pool.Exec(t.Context(), `UPDATE workspace_settings SET value=$1::jsonb WHERE key='api_secrets'`, value)
		if err == nil || !strings.Contains(err.Error(), "workspace_settings_retired_api_secret_absent") {
			t.Errorf("retired/malformed API secrets %s update error=%v", value, err)
		}
	}
	assertAppliedMigrationCount(t, pool, retiredExecutionAuthorityMigration, 1)
}

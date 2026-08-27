package queue

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresRetiredExecutionAuthorityRejectsContaminationAtomically(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "090")); err != nil {
		t.Fatal(err)
	}
	project, err := repository.CreateProject(t.Context(), "Retirement preflight", "/srv/retirement-preflight", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateScrumCard(
		t.Context(), project.ID, "preflight-card", "Preflight card", "", "assigned",
		json.RawMessage(`[]`), json.RawMessage(`[]`),
	); err != nil {
		t.Fatal(err)
	}
	fixture := seedPreInlineExecutionMigrationJob(
		t, t.Context(), pool, "retirement preflight job", "coding", "v3_coding", nil,
	)

	tests := []struct {
		name, setup, cleanup, want string
	}{
		{"project recipe id", `UPDATE projects SET recipe_id='retired' WHERE id=` + integerSQL(project.ID), `UPDATE projects SET recipe_id='' WHERE id=` + integerSQL(project.ID), "retired project"},
		{"project recipe", `UPDATE projects SET recipe='{"retired":true}' WHERE id=` + integerSQL(project.ID), `UPDATE projects SET recipe='{}' WHERE id=` + integerSQL(project.ID), "retired project"},
		{"project agent config", `UPDATE projects SET settings='{"agent_config":{}}' WHERE id=` + integerSQL(project.ID), `UPDATE projects SET settings='{}' WHERE id=` + integerSQL(project.ID), "retired project"},
		{"project malformed settings", `UPDATE projects SET settings='null' WHERE id=` + integerSQL(project.ID), `UPDATE projects SET settings='{}' WHERE id=` + integerSQL(project.ID), "retired project"},
		{"card agent config", `UPDATE scrum_cards SET agent_config='{"agent_system":"omnidex"}' WHERE id='preflight-card'`, `UPDATE scrum_cards SET agent_config='{}' WHERE id='preflight-card'`, "retired Scrum card"},
		{"card model config", `UPDATE scrum_cards SET model_config='{"retired":"model"}' WHERE id='preflight-card'`, `UPDATE scrum_cards SET model_config='{}' WHERE id='preflight-card'`, "retired Scrum card"},
		{"card coach config", `UPDATE scrum_cards SET coach_config='{"retired":true}' WHERE id='preflight-card'`, `UPDATE scrum_cards SET coach_config='{}' WHERE id='preflight-card'`, "retired Scrum card"},
		{"card recipe id", `UPDATE scrum_cards SET recipe_id='retired' WHERE id='preflight-card'`, `UPDATE scrum_cards SET recipe_id='' WHERE id='preflight-card'`, "retired Scrum card"},
		{"card recipe", `UPDATE scrum_cards SET recipe='{"retired":true}' WHERE id='preflight-card'`, `UPDATE scrum_cards SET recipe='{}' WHERE id='preflight-card'`, "retired Scrum card"},
		{"card tags job", `UPDATE scrum_cards SET tags_job_id='7' WHERE id='preflight-card'`, `UPDATE scrum_cards SET tags_job_id='' WHERE id='preflight-card'`, "retired Scrum card"},
		{"card ticket job", `UPDATE scrum_cards SET ticket_job_id='8' WHERE id='preflight-card'`, `UPDATE scrum_cards SET ticket_job_id='' WHERE id='preflight-card'`, "retired Scrum card"},
		{"workspace agent row", `INSERT INTO workspace_settings(key,value) VALUES('workspace_agent_config','{}')`, `DELETE FROM workspace_settings WHERE key='workspace_agent_config'`, "workspace agent"},
		{"cursor API secret", `INSERT INTO workspace_settings(key,value) VALUES('api_secrets','{"cursor_api_key":"retired"}')`, `DELETE FROM workspace_settings WHERE key='api_secrets'`, "API secrets"},
		{"codex API secret", `INSERT INTO workspace_settings(key,value) VALUES('api_secrets','{"codex_api_key":"retired"}')`, `DELETE FROM workspace_settings WHERE key='api_secrets'`, "API secrets"},
		{"null API secrets", `INSERT INTO workspace_settings(key,value) VALUES('api_secrets','null')`, `DELETE FROM workspace_settings WHERE key='api_secrets'`, "API secrets"},
		{"array API secrets", `INSERT INTO workspace_settings(key,value) VALUES('api_secrets','[]')`, `DELETE FROM workspace_settings WHERE key='api_secrets'`, "API secrets"},
		{"job agent metadata", `UPDATE jobs SET metadata=jsonb_set(metadata,'{agent_config}','{"agent_system":"omnidex"}') WHERE id=` + integerSQL(fixture.Job.ID), `UPDATE jobs SET metadata=metadata-'agent_config' WHERE id=` + integerSQL(fixture.Job.ID), "execution metadata"},
		{"job instance metadata", `UPDATE jobs SET metadata=jsonb_set(metadata,'{instance_agent_config}','{}') WHERE id=` + integerSQL(fixture.Job.ID), `UPDATE jobs SET metadata=metadata-'instance_agent_config' WHERE id=` + integerSQL(fixture.Job.ID), "execution metadata"},
		{"job recipe metadata", `UPDATE jobs SET metadata=jsonb_set(metadata,'{recipe}','{}') WHERE id=` + integerSQL(fixture.Job.ID), `UPDATE jobs SET metadata=metadata-'recipe' WHERE id=` + integerSQL(fixture.Job.ID), "execution metadata"},
		{"job malformed metadata", `UPDATE jobs SET metadata='null' WHERE id=` + integerSQL(fixture.Job.ID), `UPDATE jobs SET metadata='{}' WHERE id=` + integerSQL(fixture.Job.ID), "execution metadata"},
		{"external agent step", `UPDATE job_steps SET action='external_agent_execute' WHERE job_id=` + integerSQL(fixture.Job.ID), `UPDATE job_steps SET action='v3_coding' WHERE job_id=` + integerSQL(fixture.Job.ID), "external-agent job steps"},
		{"telemetry recipe", `UPDATE omni_runs SET recipe_id='retired' WHERE id=(SELECT id FROM omni_runs ORDER BY id LIMIT 1)`, `UPDATE omni_runs SET recipe_id='' WHERE id=(SELECT id FROM omni_runs ORDER BY id LIMIT 1)`, "execution telemetry"},
		{"telemetry external agent", `UPDATE omni_runs SET external_agents_used=ARRAY['codex'] WHERE id=(SELECT id FROM omni_runs ORDER BY id LIMIT 1)`, `UPDATE omni_runs SET external_agents_used=ARRAY[]::TEXT[] WHERE id=(SELECT id FROM omni_runs ORDER BY id LIMIT 1)`, "execution telemetry"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if tag, err := pool.Exec(t.Context(), test.setup); err != nil {
				t.Fatalf("seed contamination: %v", err)
			} else if tag.RowsAffected() != 1 {
				t.Fatalf("seed contamination changed %d rows", tag.RowsAffected())
			}
			err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "091"))
			if err == nil || !strings.Contains(err.Error(), "migration 091 reset required") ||
				!strings.Contains(err.Error(), test.want) {
				t.Fatalf("cutover error=%v want reset-required %q", err, test.want)
			}
			assertRetiredExecutionCatalogUnchanged(t, pool)
			assertAppliedMigrationCount(t, pool, retiredExecutionAuthorityMigration, 0)
			if tag, err := pool.Exec(t.Context(), test.cleanup); err != nil {
				t.Fatalf("clean contamination: %v", err)
			} else if tag.RowsAffected() != 1 {
				t.Fatalf("clean contamination changed %d rows", tag.RowsAffected())
			}
		})
	}
}

func assertRetiredExecutionCatalogUnchanged(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var columnCount, functionCount int
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema=current_schema() AND (
		  (table_name='projects' AND column_name IN ('recipe_id','recipe')) OR
		  (table_name='scrum_cards' AND column_name IN
		   ('agent_config','model_config','coach_config','recipe_id','recipe','tags_job_id','ticket_job_id')) OR
		  (table_name='omni_runs' AND column_name IN ('recipe_id','external_agents_used')))
	`).Scan(&columnCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT COUNT(*) FROM pg_proc AS p JOIN pg_namespace AS n ON n.oid=p.pronamespace WHERE n.nspname=current_schema() AND p.proname='omni_valid_agent_config'`).Scan(&functionCount); err != nil {
		t.Fatal(err)
	}
	if columnCount != 11 || functionCount != 1 {
		t.Fatalf("rejected cutover changed retired catalog columns/functions=%d/%d", columnCount, functionCount)
	}
}

func integerSQL(value int64) string {
	return strconv.FormatInt(value, 10)
}

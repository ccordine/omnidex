package queue

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPostgresRoleplaySemanticModelRouteMigratesProjectAndHistoricalJob(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "153"),
	); err != nil {
		t.Fatal(err)
	}
	project, err := repository.CreateProject(
		t.Context(), "Roleplay route migration", "/srv/workspaces/roleplay-route-migration", "",
	)
	if err != nil {
		t.Fatal(err)
	}
	legacyConfig := `{"conversation_response_model":"kept","roleplay_canon_extraction_model":"roleplay-safe","roleplay_ongoing_action_model":"roleplay-safe"}`
	if _, err := pool.Exec(t.Context(), `
		UPDATE projects
		SET settings=jsonb_build_object('model_config',$2::jsonb)
		WHERE id=$1
	`, project.ID, legacyConfig); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	var jobID int64
	if err := tx.QueryRow(t.Context(), `
		INSERT INTO jobs (
			instruction,pipeline,project_id,status,error,metadata,current_generation,completed_at
		) VALUES (
			'historical roleplay turn','chat',$1,'failed','historical provider failure',
			jsonb_build_object('model_config',$2::jsonb),1,NOW()
		) RETURNING id
	`, project.ID, legacyConfig).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `
		INSERT INTO job_generations (job_id,generation,purpose)
		VALUES ($1,1,'initial')
	`, jobID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "156"),
	); err != nil {
		t.Fatal(err)
	}
	var projectConfig, jobConfig json.RawMessage
	if err := pool.QueryRow(t.Context(), `
		SELECT settings->'model_config' FROM projects WHERE id=$1
	`, project.ID).Scan(&projectConfig); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `
		SELECT metadata->'model_config' FROM jobs WHERE id=$1
	`, jobID).Scan(&jobConfig); err != nil {
		t.Fatal(err)
	}
	for label, raw := range map[string]json.RawMessage{
		"project": projectConfig, "historical job": jobConfig,
	} {
		config, err := modelConfigMap(raw)
		if err != nil {
			t.Fatalf("%s model config: %v", label, err)
		}
		if config["roleplay_semantic_model"] != "roleplay-safe" ||
			config["conversation_response_model"] != "kept" || len(config) != 2 {
			t.Fatalf("%s model config=%v", label, config)
		}
	}
	if _, err := pool.Exec(t.Context(), `
		UPDATE jobs
		SET metadata=jsonb_set(
			metadata,'{model_config,roleplay_semantic_model}','"changed"'::jsonb
		)
		WHERE id=$1
	`, jobID); err == nil || !strings.Contains(err.Error(), "model_config") {
		t.Fatalf("restored chat model immutability error=%v", err)
	}
}

func TestPostgresRoleplaySemanticModelRouteRejectsAmbiguousSplitValues(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "155"),
	); err != nil {
		t.Fatal(err)
	}
	project, err := repository.CreateProject(
		t.Context(), "Ambiguous roleplay route", "/srv/workspaces/ambiguous-roleplay-route", "",
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		UPDATE projects SET settings=jsonb_build_object(
			'model_config',jsonb_build_object(
				'roleplay_canon_extraction_model','canon-model',
				'roleplay_ongoing_action_model','action-model'
			)
		) WHERE id=$1
	`, project.ID); err != nil {
		t.Fatal(err)
	}
	err = repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "156"),
	)
	if err == nil || !strings.Contains(err.Error(), "conflicting split roleplay model routes") {
		t.Fatalf("ambiguous route migration error=%v", err)
	}
	var installed int
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM schema_migrations
		WHERE filename='156_roleplay_semantic_model_route.sql'
	`).Scan(&installed); err != nil {
		t.Fatal(err)
	}
	if installed != 0 {
		t.Fatalf("rejected ambiguous migration recorded %d ledger rows", installed)
	}
}

func modelConfigMap(raw json.RawMessage) (map[string]string, error) {
	var value map[string]string
	err := json.Unmarshal(raw, &value)
	return value, err
}

package queue

import (
	"strings"
	"testing"
)

func TestPostgresProjectPlanningRetirementDropsOnlyEmptyObsoleteTables(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "085")); err != nil {
		t.Fatal(err)
	}
	project, err := repository.CreateProject(
		t.Context(), "Default planning config", "/srv/default-planning-config", "", "", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO project_planning_configs(project_id,model,reasoning_mode)
		VALUES ($1,'','instant')
	`, project.ID); err != nil {
		t.Fatal(err)
	}
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "086")); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{
		"project_planning_messages", "project_planning_drafts", "project_planning_configs",
	} {
		var relation *string
		if err := pool.QueryRow(t.Context(), `SELECT to_regclass(current_schema() || '.' || $1)::text`, table).Scan(&relation); err != nil {
			t.Fatal(err)
		}
		if relation != nil {
			t.Fatalf("retired planning table %s still exists as %s", table, *relation)
		}
	}
	var ledgerCount int
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM schema_migrations
		WHERE filename='086_project_planning_retirement.sql'
	`).Scan(&ledgerCount); err != nil {
		t.Fatal(err)
	}
	if ledgerCount != 1 {
		t.Fatalf("retirement ledger count=%d want 1", ledgerCount)
	}
}

func TestPostgresProjectPlanningRetirementRefusesNonDefaultConfigAtomically(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "085")); err != nil {
		t.Fatal(err)
	}
	project, err := repository.CreateProject(
		t.Context(), "Configured planning state", "/srv/configured-planning-state", "", "", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO project_planning_configs(project_id,model,reasoning_mode)
		VALUES ($1,'legacy-model','thinking')
	`, project.ID); err != nil {
		t.Fatal(err)
	}
	err = repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "086"))
	if err == nil || !strings.Contains(err.Error(), "non-default configuration rows") {
		t.Fatalf("configured planning migration error=%v", err)
	}
	var model, reasoning string
	if err := pool.QueryRow(t.Context(), `
		SELECT model,reasoning_mode FROM project_planning_configs WHERE project_id=$1
	`, project.ID).Scan(&model, &reasoning); err != nil {
		t.Fatal(err)
	}
	if model != "legacy-model" || reasoning != "thinking" {
		t.Fatalf("rejected retirement changed config model=%q reasoning=%q", model, reasoning)
	}
}

func TestPostgresProjectPlanningRetirementRefusesRetainedStateAtomically(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "085")); err != nil {
		t.Fatal(err)
	}
	project, err := repository.CreateProject(
		t.Context(), "Retained planning state", "/srv/retained-planning-state", "", "", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO project_planning_messages(project_id,role,content)
		VALUES ($1,'user','Retain this historical planning message')
	`, project.ID); err != nil {
		t.Fatal(err)
	}
	err = repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "086"))
	if err == nil || !strings.Contains(err.Error(), "export or explicitly discard") {
		t.Fatalf("retained planning migration error=%v", err)
	}
	var messageCount, tableCount, ledgerCount int
	if err := pool.QueryRow(t.Context(), `SELECT COUNT(*) FROM project_planning_messages`).Scan(&messageCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema=current_schema()
		  AND table_name IN ('project_planning_messages','project_planning_drafts','project_planning_configs')
	`).Scan(&tableCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM schema_migrations
		WHERE filename='086_project_planning_retirement.sql'
	`).Scan(&ledgerCount); err != nil {
		t.Fatal(err)
	}
	if messageCount != 1 || tableCount != 3 || ledgerCount != 0 {
		t.Fatalf("rejected retirement changed state: messages=%d tables=%d ledger=%d", messageCount, tableCount, ledgerCount)
	}
}

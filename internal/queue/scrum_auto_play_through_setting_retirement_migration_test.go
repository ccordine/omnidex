package queue

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

const scrumAutoPlayThroughSettingRetirementMigration = "176_scrum_auto_play_through_setting_retirement.sql"

func TestScrumAutoPlayThroughSettingRetirementMigrationIsFailLoud(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + scrumAutoPlayThroughSettingRetirementMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"LOCK TABLE projects IN ACCESS EXCLUSIVE MODE",
		"WHERE settings ? 'scrum_auto_play_through'",
		"scrum auto play-through setting retirement requires a fresh reset",
		"ADD CONSTRAINT projects_removed_scrum_auto_play_through_setting",
		"NOT (settings ? 'scrum_auto_play_through')",
		"scrum auto play-through setting retirement postcondition failed",
		"scrum auto play-through setting retirement guard postcondition failed",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("Scrum auto play-through setting retirement omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"IF NOT EXISTS", "UPDATE projects", "DELETE FROM projects", "settings - 'scrum_auto_play_through'",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("Scrum auto play-through setting retirement contains fallback authority %q", forbidden)
		}
	}
}

func TestPostgresScrumAutoPlayThroughSettingRetirementRejectsDirtyState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "175")); err != nil {
		t.Fatal(err)
	}
	project, err := repository.CreateProject(ctx, "Retained obsolete Scrum setting", t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE projects
		SET settings=settings || '{"scrum_auto_play_through":true}'::JSONB
		WHERE id=$1
	`, project.ID); err != nil {
		t.Fatal(err)
	}

	err = repository.EnsureSchema(ctx, loadCheckedMigrationBundle(t))
	if err == nil || !strings.Contains(err.Error(), "requires a fresh reset") {
		t.Fatalf("dirty Scrum auto play-through setting migration error=%v", err)
	}
	assertAppliedMigrationCount(t, pool, scrumAutoPlayThroughSettingRetirementMigration, 0)
	var retained bool
	if err := pool.QueryRow(ctx, `
		SELECT settings ? 'scrum_auto_play_through'
		FROM projects
		WHERE id=$1
	`, project.ID).Scan(&retained); err != nil {
		t.Fatal(err)
	}
	if !retained {
		t.Fatal("rejected setting retirement silently removed retained dirty state")
	}
}

func TestPostgresFreshSchemaRejectsScrumAutoPlayThroughSetting(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, scrumAutoPlayThroughSettingRetirementMigration, 1)
	project, err := repository.CreateProject(ctx, "Guarded Scrum setting", t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `
		UPDATE projects
		SET settings=settings || '{"scrum_auto_play_through":false}'::JSONB
		WHERE id=$1
	`, project.ID)
	if err == nil || !strings.Contains(err.Error(), "projects_removed_scrum_auto_play_through_setting") {
		t.Fatalf("schema accepted retired Scrum auto play-through setting: %v", err)
	}
}

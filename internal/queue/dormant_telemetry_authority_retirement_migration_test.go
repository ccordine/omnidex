package queue

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

const dormantTelemetryAuthorityRetirementMigration = "173_dormant_telemetry_authority_retirement.sql"

func TestDormantTelemetryAuthorityRetirementMigrationIsExact(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + dormantTelemetryAuthorityRetirementMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, retired := range []string{
		"omni_tool_calls", "omni_command_observations", "omni_objective_metrics",
		"omni_recovery_metrics", "omni_playbook_usage", "omni_benchmark_results",
	} {
		if !strings.Contains(source, "DROP TABLE "+retired+";") {
			t.Errorf("telemetry retirement omits exact drop for %s", retired)
		}
	}
	for _, required := range []string{
		"dormant telemetry authority retirement requires a fresh reset",
		"EXISTS (SELECT 1 FROM omni_tool_calls)",
		"ALTER TABLE omni_runs", "DROP COLUMN playbook_id", "retired telemetry relation % remains",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("telemetry retirement omits %q", required)
		}
	}
	for _, forbidden := range []string{"DROP TABLE IF EXISTS", "DROP COLUMN IF EXISTS", "CASCADE", "UPDATE ", "DELETE FROM"} {
		if strings.Contains(source, forbidden) {
			t.Errorf("telemetry retirement contains fallback authority %q", forbidden)
		}
	}
}

func TestDormantTelemetryAuthorityRetirementDatabaseShape(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool := openIsolatedMigrationPool(t)
	if err := New(pool).EnsureSchema(ctx, loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, dormantTelemetryAuthorityRetirementMigration, 1)

	for _, retired := range []string{
		"omni_tool_calls", "omni_command_observations", "omni_objective_metrics",
		"omni_recovery_metrics", "omni_playbook_usage", "omni_benchmark_results",
	} {
		assertMigrationRelationExists(t, pool, retired, false)
	}
	var retiredColumn bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema=current_schema()
			  AND table_name='omni_runs' AND column_name='playbook_id'
		)
	`).Scan(&retiredColumn); err != nil {
		t.Fatal(err)
	}
	if retiredColumn {
		t.Fatal("retired omni_runs.playbook_id column remains")
	}
}

func TestDormantTelemetryAuthorityRetirementRequiresFreshState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "172")); err != nil {
		t.Fatal(err)
	}
	job, err := repository.EnqueueJob(ctx, "retained obsolete telemetry", model.PipelineCoding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO omni_tool_calls (run_id, tool_kind)
		SELECT (metadata->>'telemetry_run_id')::uuid, 'retired_probe'
		FROM jobs WHERE id=$1
	`, job.ID); err != nil {
		t.Fatal(err)
	}

	err = repository.EnsureSchema(ctx, loadCheckedMigrationBundle(t))
	if err == nil || !strings.Contains(err.Error(), "requires a fresh reset") {
		t.Fatalf("telemetry retirement error=%v, want fresh-reset failure", err)
	}
	assertAppliedMigrationCount(t, pool, dormantTelemetryAuthorityRetirementMigration, 0)
	assertMigrationRelationExists(t, pool, "omni_tool_calls", true)
}

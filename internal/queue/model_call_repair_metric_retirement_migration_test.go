package queue

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

const modelCallRepairMetricRetirementMigration = "174_model_call_repair_metric_retirement.sql"

func TestModelCallRepairMetricRetirementMigrationIsExact(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + modelCallRepairMetricRetirementMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"LOCK TABLE omni_model_calls IN ACCESS EXCLUSIVE MODE",
		"WHERE malformed OR repaired",
		"model-call repair metric retirement requires a fresh reset",
		"DROP COLUMN malformed",
		"DROP COLUMN repaired",
		"retired model-call repair metric columns remain",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("model-call metric retirement omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"DROP COLUMN IF EXISTS", "CASCADE", "UPDATE ", "DELETE FROM", "TRUNCATE",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("model-call metric retirement contains forbidden fallback %q", forbidden)
		}
	}
}

func TestPostgresModelCallRepairMetricRetirementRequiresFreshState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "173")); err != nil {
		t.Fatal(err)
	}
	job, err := repository.EnqueueJob(ctx, "retained obsolete model metric", model.PipelineCoding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO omni_model_calls (run_id, role, model, malformed)
		SELECT (metadata->>'telemetry_run_id')::uuid, 'retired_probe', 'retired_probe', true
		FROM jobs WHERE id=$1
	`, job.ID); err != nil {
		t.Fatal(err)
	}

	err = repository.EnsureSchema(ctx, loadCheckedMigrationBundle(t))
	if err == nil || !strings.Contains(err.Error(), "requires a fresh reset") {
		t.Fatalf("model-call metric retirement error=%v, want fresh-reset failure", err)
	}
	assertAppliedMigrationCount(t, pool, modelCallRepairMetricRetirementMigration, 0)
}

func TestPostgresFreshSchemaHasNoModelCallRepairMetrics(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool := openIsolatedMigrationPool(t)
	if err := New(pool).EnsureSchema(ctx, loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, modelCallRepairMetricRetirementMigration, 1)
	var count int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM information_schema.columns
		WHERE table_schema=current_schema()
		  AND table_name='omni_model_calls'
		  AND column_name IN ('malformed','repaired')
	`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("retired model-call metric columns=%d want 0", count)
	}
}

package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLifecycleAuthorityChangeFailsClosedWithoutDeploymentRail(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "116"),
	); err != nil {
		t.Fatal(err)
	}
	job, err := repository.EnqueueJob(
		t.Context(), "missing-deployment-authority-rail", model.PipelineCoding, []byte(`{}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = repository.CancelJob(
		t.Context(), testCancelCommand(t, job.ID, "missing-deployment-rail", "must fail closed"),
	)
	if err == nil || !strings.Contains(err.Error(), "generated_workload_deployments") {
		t.Fatalf("missing deployment rail cancellation error=%v", err)
	}
	var status string
	if err := pool.QueryRow(t.Context(), `SELECT status FROM jobs WHERE id=$1`, job.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != model.JobStatusPending {
		t.Fatalf("missing deployment rail cancellation changed job status=%q", status)
	}
}

// installEmptyGeneratedDeploymentAuthorityRailForHistoricalRuntimeTest lets
// tests exercise the current lifecycle API against a deliberately old schema
// without weakening the production fail-closed query. These fixtures never
// upgrade through migration 140, so the test-only relation cannot mask or
// collide with the real deployment journal migration.
func installEmptyGeneratedDeploymentAuthorityRailForHistoricalRuntimeTest(
	t *testing.T,
	pool *pgxpool.Pool,
) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `
		CREATE TABLE generated_workload_deployments (
		 id TEXT PRIMARY KEY,
		 job_id BIGINT NOT NULL,
		 generation BIGINT NOT NULL,
		 status TEXT NOT NULL
		)
	`); err != nil {
		t.Fatalf("install empty generated deployment authority rail for historical runtime test: %v", err)
	}
}

package queue

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresLegacyCognitionRetirementPreservesLiveAuthority(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	through064 := loadMigrationBundleThroughPrefix(t, "064")
	if err := repository.EnsureSchema(ctx, through064); err != nil {
		t.Fatal(err)
	}

	marker := fmt.Sprintf("legacy-retirement-live-%d", time.Now().UnixNano())
	authority, projection := seedContextProjectionTest(t, ctx, repository, pool, marker)
	if _, err := repository.StoreContextProjection(ctx, authority, projection); err != nil {
		t.Fatal(err)
	}
	before := legacyRetirementMigrationLedgerSnapshot(t, ctx, pool)

	through065 := loadMigrationBundleThroughPrefix(t, "065")
	if err := repository.EnsureSchema(ctx, through065); err != nil {
		t.Fatal(err)
	}

	after := legacyRetirementMigrationLedgerSnapshotBefore065(t, ctx, pool)
	if before != after {
		t.Fatalf("065 rewrote historical migration filename/body/time authority:\nbefore=%s\nafter=%s", before, after)
	}
	assertExactMigrationLedger(t, pool, through065)
	assertAppliedMigrationCount(t, pool, "065_legacy_cognition_runtime_retirement.sql", 1)
	assertLegacyCognitionCatalogRetired(t, ctx, pool)
	assertLiveOnlyContextProjectionConstraint(t, ctx, pool)
	if _, err := repository.GetContextProjection(ctx, projection.ID); err != nil {
		t.Fatalf("load preserved live context projection: %v", err)
	}
	var liveRows int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM context_projections
		WHERE projection_id=$1 AND usage_mode='live'
	`, projection.ID).Scan(&liveRows); err != nil {
		t.Fatal(err)
	}
	if liveRows != 1 {
		t.Fatalf("preserved live context projection rows=%d want 1", liveRows)
	}
}

func TestPostgresLifecycleWorksWithoutLegacyCognitionSeals(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	assertLegacyCognitionCatalogRetired(t, ctx, pool)
	marker := fmt.Sprintf("post-cognition-lifecycle-%d", time.Now().UnixNano())

	cancelJob, err := repository.EnqueueJob(ctx, marker+"-cancel", model.PipelineCoding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	cancel := testCancelCommand(t, cancelJob.ID, "without-cognition-seal", "ordinary cancellation")
	canceled, err := repository.CancelJob(ctx, cancel)
	if err != nil {
		t.Fatal(err)
	}
	if canceled.Status != model.JobStatusCanceled {
		t.Fatalf("canceled status=%q want %q", canceled.Status, model.JobStatusCanceled)
	}
	if _, err := repository.CancelJob(ctx, cancel); err != nil {
		t.Fatalf("replay cancellation without cognition seal: %v", err)
	}

	replanJob, err := repository.EnqueueJob(ctx, marker+"-replan", model.PipelineCoding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	replan := testReplanCommand(t, replanJob.ID, "without-cognition-seal", "replace the pending tail")
	replanned, err := repository.ReplanJob(ctx, replan)
	if err != nil {
		t.Fatal(err)
	}
	if replanned.ID != replanJob.ID || replanned.CurrentGeneration != 2 {
		t.Fatalf("replanned job=%+v", replanned)
	}
	if _, err := repository.ReplanJob(ctx, replan); err != nil {
		t.Fatalf("replay replan without cognition seal: %v", err)
	}

	var operations int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM job_lifecycle_operations
		WHERE operation_id=ANY($1::text[])
	`, []string{string(cancel.OperationID), string(replan.OperationID)}).Scan(&operations); err != nil {
		t.Fatal(err)
	}
	if operations != 2 {
		t.Fatalf("lifecycle operation rows=%d want 2", operations)
	}
}

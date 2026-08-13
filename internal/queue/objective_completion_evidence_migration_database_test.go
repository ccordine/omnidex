package queue

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresObjectiveCompletionMigrationRejectsUnauthenticatedHistoryAtomically(t *testing.T) {
	ctx := context.Background()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "079")); err != nil {
		t.Fatal(err)
	}
	job, err := repository.EnqueueJob(ctx, "legacy objective citation", model.PipelineCoding, nil)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "legacy-objective-worker")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%+v err=%v", claim, err)
	}
	record := objectiveCompletionEvidence(claim, "legacy-sidecar")
	payload, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO evidence (job_id,step_id,kind,source_type,source_ref,payload_json)
		VALUES ($1,$2,$3,$4,$5,$6::jsonb)
	`, record.JobID, record.StepID, evidence.KindObjectiveCitation,
		record.SourceType, record.SourceRef, string(payload)); err != nil {
		t.Fatal(err)
	}
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "080")); err == nil {
		t.Fatal("migration accepted unauthenticated objective citation history")
	}
	assertAppliedMigrationCount(t, pool, "080_objective_completion_evidence_authority.sql", 0)
	var count int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM evidence WHERE kind='objective_citation'
	`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("legacy evidence count=%d want 1 after atomic refusal", count)
	}
}

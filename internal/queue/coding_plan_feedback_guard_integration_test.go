package queue

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestCodingPlanReviewCannotBeCompletedByGenericFeedback(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	leaf := codingPlanExecutableLeaf(
		t,
		"A user can confirm the item.",
		model.CodingPlanDecisionPending,
		1,
	)
	job, _, plan := storeCodingPlanFixture(t, repository, []CodingPlanLeafWrite{leaf})
	workspaceRoot, workspaceIdentity := codingPlanWorkspaceAuthority(t, job)

	_, err := repository.SubmitJobFeedback(ctx, SubmitJobFeedbackCommand{
		OperationID:   codingPlanOperationID(t, "generic-feedback", job.ID),
		JobID:         job.ID,
		Feedback:      "Approve this work.",
		WorkspaceRoot: workspaceRoot, WorkspaceIdentity: workspaceIdentity,
	})
	if !errors.Is(err, ErrStepNotWritable) {
		t.Fatalf("generic plan feedback error = %v, want %v", err, ErrStepNotWritable)
	}
	requireCodingPlanStillWaiting(t, repository, job.ID, plan)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin direct plan-step mutation: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status=$3,finished_at=clock_timestamp(),updated_at=clock_timestamp()
		WHERE job_id=$1 AND generation=$2 AND action='v3_coding_plan'
	`, job.ID, plan.Generation, model.StepStatusCompleted); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("stage direct plan-step completion: %v", err)
	}
	if err := tx.Commit(ctx); err == nil {
		t.Fatal("database committed a coding-plan step completion before plan freeze")
	}
	requireCodingPlanStillWaiting(t, repository, job.ID, plan)
}

func TestCodingPlanStepCannotCompleteWithoutPersistedPlan(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "Build the exact coding-plan fixture.", t.TempDir())
	if err != nil {
		t.Fatalf("enqueue coding-plan fixture: %v", err)
	}
	claim, err := repository.ClaimNextStep(ctx, "coding-plan-no-plan-fixture")
	if err != nil {
		t.Fatalf("claim coding-plan fixture: %v", err)
	}
	if claim == nil || claim.Job.ID != job.ID || claim.Step.Action != "v3_coding_plan" {
		t.Fatalf("coding-plan fixture claim = %#v", claim)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin missing-plan step mutation: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status=$3,finished_at=clock_timestamp(),updated_at=clock_timestamp()
		WHERE job_id=$1 AND generation=$2 AND action='v3_coding_plan'
	`, job.ID, int64(1), model.StepStatusCompleted); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("stage missing-plan completion: %v", err)
	}
	if err := tx.Commit(ctx); err == nil {
		t.Fatal("database completed a coding-plan step without a persisted plan")
	}

	details, err := repository.CurrentJobDetails(ctx, job.ID)
	if err != nil {
		t.Fatalf("read job after rejected missing-plan completion: %v", err)
	}
	if details.Job.Status != model.JobStatusRunning || len(details.Steps) != 2 ||
		details.Steps[0].Status != model.StepStatusRunning ||
		details.Steps[1].Status != model.StepStatusPending {
		t.Fatalf("missing-plan completion changed authority: %#v", details)
	}
}

func requireCodingPlanStillWaiting(
	t *testing.T,
	repository *Repository,
	jobID int64,
	wantPlan model.CodingPlan,
) {
	t.Helper()
	ctx := context.Background()
	current, err := repository.CurrentCodingPlan(ctx, jobID)
	if err != nil {
		t.Fatalf("read plan after rejected completion: %v", err)
	}
	if !sameCodingPlanProjection(current, wantPlan) {
		t.Fatalf("plan changed after rejected completion: %#v", current)
	}
	details, err := repository.CurrentJobDetails(ctx, jobID)
	if err != nil {
		t.Fatalf("read job after rejected completion: %v", err)
	}
	if details.Job.Status != model.JobStatusWaiting || len(details.Steps) != 2 ||
		details.Steps[0].Action != "v3_coding_plan" ||
		details.Steps[0].Status != model.StepStatusWaiting ||
		details.Steps[1].Action != "v3_coding" ||
		details.Steps[1].Status != model.StepStatusPending {
		t.Fatalf("plan review advanced without freeze: %#v", details)
	}
	if claim, err := repository.ClaimNextStep(ctx, "unfrozen-plan-claim"); err != nil || claim != nil {
		t.Fatalf("unfrozen plan produced execution claim %#v, error %v", claim, err)
	}
}

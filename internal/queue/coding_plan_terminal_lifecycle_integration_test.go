package queue

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestCodingPlanInterruptSupersedesFrozenAuthority(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	leaf := codingPlanExecutableLeaf(
		t, "A user can confirm the item.", model.CodingPlanDecisionPending, 1,
	)
	job, _, plan := storeCodingPlanFixture(t, repository, []CodingPlanLeafWrite{leaf})
	frozen := approveAndFreezeCodingPlan(t, repository, job, plan, leaf.Leaf.ID)
	workspaceRoot, workspaceIdentity := codingPlanWorkspaceAuthority(t, job)

	result, err := repository.InterruptJob(ctx, ReplanJobCommand{
		OperationID:   codingPlanOperationID(t, "interrupt-frozen-plan", job.ID),
		JobID:         job.ID,
		Feedback:      "Pause before executing the approved plan.",
		WorkspaceRoot: workspaceRoot, WorkspaceIdentity: workspaceIdentity,
	})
	if err != nil {
		t.Fatalf("interrupt frozen coding plan: %v", err)
	}
	if !result.Applied || result.Job.ID != job.ID || result.Job.CurrentGeneration != 2 ||
		result.Job.Status != model.JobStatusWaiting {
		t.Fatalf("interrupt result = %#v", result)
	}
	retired, err := readCodingPlan(ctx, pool, job.ID, 1)
	if err != nil {
		t.Fatalf("read interrupted coding plan: %v", err)
	}
	if retired.State != model.CodingPlanStateSuperseded ||
		retired.Revision != frozen.Revision+1 || retired.FrozenAt == nil {
		t.Fatalf("interrupted plan = %#v, want frozen authority superseded once", retired)
	}
	var status string
	var supersededAt *int64
	if err := pool.QueryRow(ctx, `
		SELECT status,superseded_at_generation
		FROM job_steps
		WHERE job_id=$1 AND generation=1 AND action='v3_coding_plan'
	`, job.ID).Scan(&status, &supersededAt); err != nil {
		t.Fatalf("read interrupted review step: %v", err)
	}
	if status != model.StepStatusCompleted || supersededAt == nil || *supersededAt != 2 {
		t.Fatalf("interrupted review step = %s/%v, want completed/superseded-at-2", status, supersededAt)
	}
}

func TestCodingPlanCancellationRetiresReviewAuthority(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	leaf := codingPlanExecutableLeaf(
		t, "A user can confirm the item.", model.CodingPlanDecisionPending, 1,
	)
	job, _, plan := storeCodingPlanFixture(t, repository, []CodingPlanLeafWrite{leaf})
	workspaceRoot, workspaceIdentity := codingPlanWorkspaceAuthority(t, job)

	result, err := repository.CancelJob(ctx, CancelJobCommand{
		OperationID:   codingPlanOperationID(t, "cancel-review-plan", job.ID),
		JobID:         job.ID,
		Reason:        "Stop this objective before approval.",
		WorkspaceRoot: workspaceRoot, WorkspaceIdentity: workspaceIdentity,
	})
	if err != nil {
		t.Fatalf("cancel review coding plan: %v", err)
	}
	if !result.Applied || result.Job.Status != model.JobStatusCanceled {
		t.Fatalf("cancel review result = %#v", result)
	}
	canceled, err := repository.CurrentCodingPlan(ctx, job.ID)
	if err != nil {
		t.Fatalf("read canceled review plan: %v", err)
	}
	if canceled.State != model.CodingPlanStateCanceled || canceled.Revision != plan.Revision+1 ||
		canceled.FrozenAt != nil || canceled.Leaves[0].Decision != model.CodingPlanDecisionPending {
		t.Fatalf("canceled review plan = %#v", canceled)
	}
	requireCanceledCodingPlanSteps(t, repository, job.ID, model.StepStatusCanceled)
}

func TestCodingPlanCancellationRetiresFrozenAuthority(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	leaf := codingPlanExecutableLeaf(
		t, "A user can confirm the item.", model.CodingPlanDecisionPending, 1,
	)
	job, _, plan := storeCodingPlanFixture(t, repository, []CodingPlanLeafWrite{leaf})
	frozen := approveAndFreezeCodingPlan(t, repository, job, plan, leaf.Leaf.ID)
	workspaceRoot, workspaceIdentity := codingPlanWorkspaceAuthority(t, job)

	result, err := repository.CancelJob(ctx, CancelJobCommand{
		OperationID:   codingPlanOperationID(t, "cancel-frozen-plan", job.ID),
		JobID:         job.ID,
		Reason:        "Stop this approved objective before execution.",
		WorkspaceRoot: workspaceRoot, WorkspaceIdentity: workspaceIdentity,
	})
	if err != nil {
		t.Fatalf("cancel frozen coding plan: %v", err)
	}
	if !result.Applied || result.Job.Status != model.JobStatusCanceled {
		t.Fatalf("cancel frozen result = %#v", result)
	}
	canceled, err := repository.CurrentCodingPlan(ctx, job.ID)
	if err != nil {
		t.Fatalf("read canceled frozen plan: %v", err)
	}
	if canceled.State != model.CodingPlanStateCanceled || canceled.Revision != frozen.Revision+1 ||
		canceled.FrozenAt == nil || canceled.Leaves[0].Decision != model.CodingPlanDecisionApproved {
		t.Fatalf("canceled frozen plan = %#v", canceled)
	}
	requireCanceledCodingPlanSteps(t, repository, job.ID, model.StepStatusCompleted)
}

func approveAndFreezeCodingPlan(
	t *testing.T,
	repository *Repository,
	job model.Job,
	plan model.CodingPlan,
	leafID model.CodingPlanLeafID,
) model.CodingPlan {
	t.Helper()
	ctx := context.Background()
	workspaceRoot, workspaceIdentity := codingPlanWorkspaceAuthority(t, job)
	decided, err := repository.ApplyCodingPlanDecisions(ctx, ApplyCodingPlanDecisionsCommand{
		OperationID:   codingPlanOperationID(t, "approve-plan", job.ID),
		JobID:         job.ID,
		Generation:    plan.Generation,
		Revision:      plan.Revision,
		WorkspaceRoot: workspaceRoot, WorkspaceIdentity: workspaceIdentity,
		Decisions: []CodingPlanDecisionChange{{
			LeafID: leafID, Decision: model.CodingPlanDecisionApproved,
		}},
	})
	if err != nil {
		t.Fatalf("approve coding plan: %v", err)
	}
	frozen, err := repository.FreezeCodingPlan(ctx, FreezeCodingPlanCommand{
		OperationID:   codingPlanOperationID(t, "freeze-plan", job.ID),
		JobID:         job.ID,
		Generation:    plan.Generation,
		Revision:      decided.Plan.Revision,
		WorkspaceRoot: workspaceRoot, WorkspaceIdentity: workspaceIdentity,
	})
	if err != nil {
		t.Fatalf("freeze coding plan: %v", err)
	}
	return frozen.Plan
}

func requireCanceledCodingPlanSteps(
	t *testing.T,
	repository *Repository,
	jobID int64,
	wantPlanStepStatus string,
) {
	t.Helper()
	details, err := repository.CurrentJobDetails(context.Background(), jobID)
	if err != nil {
		t.Fatalf("read canceled coding-plan job: %v", err)
	}
	if details.Job.Status != model.JobStatusCanceled || len(details.Steps) != 2 ||
		details.Steps[0].Action != "v3_coding_plan" ||
		details.Steps[0].Status != wantPlanStepStatus ||
		details.Steps[1].Action != "v3_coding" ||
		details.Steps[1].Status != model.StepStatusCanceled {
		t.Fatalf("canceled coding-plan authority = %#v", details)
	}
}

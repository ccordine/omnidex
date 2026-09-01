package queue

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

func TestCodingPlanReviewDecisionFreezeAndExecutionPersistence(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	ctx := context.Background()

	approvedWrite := codingPlanExecutableLeaf(
		t, "A user can confirm the item.", model.CodingPlanDecisionPending, 1,
	)
	rejectedWrite := codingPlanExecutableLeaf(
		t, "The confirmed state remains visible.", model.CodingPlanDecisionPending, 1,
	)
	conflictWrite := codingPlanConflictLeaf(t, "Export every confirmation to an unrelated cloud service.", 1)
	job, claim, plan := storeCodingPlanFixture(
		t, repository, []CodingPlanLeafWrite{approvedWrite, rejectedWrite, conflictWrite},
	)
	workspaceRoot, workspaceIdentity := codingPlanWorkspaceAuthority(t, job)
	if plan.JobID != job.ID || plan.Generation != 1 || plan.Revision != 1 ||
		plan.State != model.CodingPlanStateReview || len(plan.Leaves) != 3 {
		t.Fatalf("persisted review plan = %#v", plan)
	}
	for index, write := range []CodingPlanLeafWrite{approvedWrite, rejectedWrite, conflictWrite} {
		if plan.Leaves[index] != write.Leaf {
			t.Fatalf("persisted leaf %d = %#v, want %#v", index, plan.Leaves[index], write.Leaf)
		}
	}
	current, err := repository.CurrentCodingPlan(ctx, job.ID)
	if err != nil {
		t.Fatalf("read current review plan: %v", err)
	}
	if current.RequestSHA256 != plan.RequestSHA256 || current.ScopeMode != model.CodingScopeModeNormal {
		t.Fatalf("current plan authority = %#v", current)
	}
	restored, err := repository.StoreCodingPlanReview(ctx, StoreCodingPlanReviewCommand{
		Authority: claim.Authority, ScopeMode: model.CodingScopeModeNormal,
		RequestSHA256: assemblyline.ExactObjectiveContextSHA(job.Instruction),
		Leaves:        []CodingPlanLeafWrite{approvedWrite, rejectedWrite, conflictWrite},
	})
	if err != nil {
		t.Fatalf("replay exact persisted review result: %v", err)
	}
	if !sameCodingPlanProjection(restored, plan) {
		t.Fatalf("restored review = %#v, want persisted %#v", restored, plan)
	}
	unpairedCommand := ApplyCodingPlanDecisionsCommand{
		OperationID: codingPlanOperationID(t, "unpaired-result", job.ID),
		JobID:       job.ID, Generation: 1, Revision: 1,
		WorkspaceRoot: workspaceRoot, WorkspaceIdentity: workspaceIdentity,
		Decisions: []CodingPlanDecisionChange{{
			LeafID: approvedWrite.Leaf.ID, Decision: model.CodingPlanDecisionApproved,
		}},
	}
	descriptor, err := describeLifecycleOperation(
		unpairedCommand.OperationID, LifecycleCodingPlanDecisions, unpairedCommand,
	)
	if err != nil {
		t.Fatalf("describe unpaired operation fixture: %v", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin unpaired operation fixture: %v", err)
	}
	if err := lockLifecycleOperationIdentityTx(ctx, tx, descriptor.ID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("lock unpaired operation identity: %v", err)
	}
	waitingJob, err := lockedJobTx(ctx, tx, job.ID)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("lock unpaired operation job: %v", err)
	}
	if _, found, err := loadLifecycleOperationTx(ctx, tx, descriptor, job.ID); err != nil || found {
		_ = tx.Rollback(ctx)
		t.Fatalf("reserve unpaired operation: found=%v err=%v", found, err)
	}
	waitingStatus := model.StepStatusWaiting
	if err := insertLifecycleOperationTx(ctx, tx, descriptor, lifecycleOperationRecord{
		ID: descriptor.ID, JobID: job.ID, ObservedGeneration: 1, ResultGeneration: 1,
		StepID: &claim.Step.ID, Kind: descriptor.Kind, CommandSHA256: descriptor.SHA256,
		ResultJobStatus: waitingJob.Status, ResultStepStatus: &waitingStatus, ResultJob: waitingJob,
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("insert unpaired coding-plan operation: %v", err)
	}
	if err := tx.Commit(ctx); err == nil {
		t.Fatal("database committed a coding-plan lifecycle operation without its exact plan result")
	}

	decisionCommand := ApplyCodingPlanDecisionsCommand{
		OperationID: codingPlanOperationID(t, "decide", job.ID),
		JobID:       job.ID, Generation: 1, Revision: 1,
		WorkspaceRoot: workspaceRoot, WorkspaceIdentity: workspaceIdentity,
		Decisions: []CodingPlanDecisionChange{
			{LeafID: approvedWrite.Leaf.ID, Decision: model.CodingPlanDecisionApproved},
			{LeafID: rejectedWrite.Leaf.ID, Decision: model.CodingPlanDecisionRejected},
			{LeafID: conflictWrite.Leaf.ID, Decision: model.CodingPlanDecisionApproved},
		},
	}
	decisionResult, err := repository.ApplyCodingPlanDecisions(ctx, decisionCommand)
	if err != nil {
		t.Fatalf("apply exact plan decisions: %v", err)
	}
	if !decisionResult.Applied || decisionResult.Plan.Revision != 2 ||
		decisionResult.Plan.Leaves[0].Decision != model.CodingPlanDecisionApproved ||
		decisionResult.Plan.Leaves[1].Decision != model.CodingPlanDecisionRejected ||
		decisionResult.Plan.Leaves[2].Decision != model.CodingPlanDecisionApproved {
		t.Fatalf("decision result = %#v", decisionResult)
	}

	staleCommand := ApplyCodingPlanDecisionsCommand{
		OperationID: codingPlanOperationID(t, "stale", job.ID),
		JobID:       job.ID, Generation: 1, Revision: 1,
		WorkspaceRoot: workspaceRoot, WorkspaceIdentity: workspaceIdentity,
		Decisions: []CodingPlanDecisionChange{{
			LeafID: approvedWrite.Leaf.ID, Decision: model.CodingPlanDecisionRejected,
		}},
	}
	if _, err := repository.ApplyCodingPlanDecisions(ctx, staleCommand); !errors.Is(err, ErrCodingPlanRevision) {
		t.Fatalf("stale revision error = %v, want %v", err, ErrCodingPlanRevision)
	}

	replay, err := repository.ApplyCodingPlanDecisions(ctx, decisionCommand)
	if err != nil {
		t.Fatalf("replay exact decision operation: %v", err)
	}
	if replay.Applied || replay.Plan.Revision != decisionResult.Plan.Revision ||
		!sameCodingPlanProjection(replay.Plan, decisionResult.Plan) {
		t.Fatalf("decision replay = %#v, want immutable %#v", replay, decisionResult)
	}

	freezeCommand := FreezeCodingPlanCommand{
		OperationID: codingPlanOperationID(t, "freeze", job.ID),
		JobID:       job.ID, Generation: 1, Revision: 2,
		WorkspaceRoot: workspaceRoot, WorkspaceIdentity: workspaceIdentity,
	}
	frozen, err := repository.FreezeCodingPlan(ctx, freezeCommand)
	if err != nil {
		t.Fatalf("freeze decided plan: %v", err)
	}
	if !frozen.Applied || frozen.Plan.State != model.CodingPlanStateFrozen ||
		frozen.Plan.Revision != 3 || frozen.Plan.FrozenAt == nil ||
		frozen.Job.Status != model.JobStatusRunning {
		t.Fatalf("frozen result = %#v", frozen)
	}
	freezeReplay, err := repository.FreezeCodingPlan(ctx, freezeCommand)
	if err != nil {
		t.Fatalf("replay exact freeze operation: %v", err)
	}
	if freezeReplay.Applied || !sameCodingPlanProjection(freezeReplay.Plan, frozen.Plan) {
		t.Fatalf("freeze replay = %#v, want immutable %#v", freezeReplay, frozen)
	}

	postFreeze := ApplyCodingPlanDecisionsCommand{
		OperationID: codingPlanOperationID(t, "post-freeze", job.ID),
		JobID:       job.ID, Generation: 1, Revision: 3,
		WorkspaceRoot: workspaceRoot, WorkspaceIdentity: workspaceIdentity,
		Decisions: []CodingPlanDecisionChange{{
			LeafID: approvedWrite.Leaf.ID, Decision: model.CodingPlanDecisionRejected,
		}},
	}
	if _, err := repository.ApplyCodingPlanDecisions(ctx, postFreeze); !errors.Is(err, ErrCodingPlanState) {
		t.Fatalf("post-freeze decision error = %v, want %v", err, ErrCodingPlanState)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE coding_plan_leaves SET decision='rejected',updated_at=clock_timestamp()
		WHERE job_id=$1 AND generation=1 AND leaf_id=$2
	`, job.ID, approvedWrite.Leaf.ID); err == nil {
		t.Fatal("database accepted a direct mutation of a frozen plan leaf")
	}

	executionClaim, err := repository.ClaimNextStep(ctx, "coding-plan-execution-fixture")
	if err != nil {
		t.Fatalf("claim frozen plan execution: %v", err)
	}
	if executionClaim == nil || executionClaim.Job.ID != job.ID ||
		executionClaim.Step.Action != "v3_coding" || executionClaim.Step.Generation != 1 {
		t.Fatalf("execution claim = %#v", executionClaim)
	}
	loaded, err := repository.LoadFrozenCodingPlan(ctx, executionClaim.Authority)
	if err != nil {
		t.Fatalf("load frozen execution authority: %v", err)
	}
	if loaded.Plan.State != model.CodingPlanStateFrozen || len(loaded.Leaves) != 2 ||
		loaded.Leaves[0].Leaf.ID != approvedWrite.Leaf.ID ||
		loaded.Leaves[0].Leaf.Decision != model.CodingPlanDecisionApproved ||
		loaded.Leaves[1].Leaf.ID != conflictWrite.Leaf.ID ||
		loaded.Leaves[1].Leaf.Decision != model.CodingPlanDecisionApproved {
		t.Fatalf("loaded frozen execution leaves = %#v", loaded)
	}
	if err := loaded.Leaves[0].ResultRelation.assemblyline().ValidateAcceptedFor(
		approvedWrite.Leaf.Statement,
	); err != nil {
		t.Fatalf("loaded execution receipt: %v", err)
	}
	if claim.Authority.JobID != executionClaim.Authority.JobID {
		t.Fatalf("execution changed job identity from %d to %d", claim.Authority.JobID, executionClaim.Authority.JobID)
	}
}

func TestCodingPlanFreezeRequiresEveryDecisionAndAnApproval(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	one := codingPlanExecutableLeaf(t, "A user can archive one item.", model.CodingPlanDecisionPending, 1)
	two := codingPlanExecutableLeaf(t, "The archived state is visible.", model.CodingPlanDecisionPending, 1)
	job, _, _ := storeCodingPlanFixture(t, repository, []CodingPlanLeafWrite{one, two})
	workspaceRoot, workspaceIdentity := codingPlanWorkspaceAuthority(t, job)
	if _, err := pool.Exec(ctx, `
		UPDATE coding_plans
		SET state='frozen',revision=revision+1,
		    updated_at=clock_timestamp(),frozen_at=clock_timestamp()
		WHERE job_id=$1 AND generation=1
	`, job.ID); err == nil {
		t.Fatal("database accepted direct freeze with pending leaves")
	}

	freezePending := FreezeCodingPlanCommand{
		OperationID: codingPlanOperationID(t, "pending-freeze", job.ID),
		JobID:       job.ID, Generation: 1, Revision: 1,
		WorkspaceRoot: workspaceRoot, WorkspaceIdentity: workspaceIdentity,
	}
	if _, err := repository.FreezeCodingPlan(ctx, freezePending); !errors.Is(err, ErrCodingPlanState) {
		t.Fatalf("pending freeze error = %v, want %v", err, ErrCodingPlanState)
	}
	decisions, err := repository.ApplyCodingPlanDecisions(ctx, ApplyCodingPlanDecisionsCommand{
		OperationID: codingPlanOperationID(t, "reject-all", job.ID),
		JobID:       job.ID, Generation: 1, Revision: 1,
		WorkspaceRoot: workspaceRoot, WorkspaceIdentity: workspaceIdentity,
		Decisions: []CodingPlanDecisionChange{
			{LeafID: one.Leaf.ID, Decision: model.CodingPlanDecisionRejected},
			{LeafID: two.Leaf.ID, Decision: model.CodingPlanDecisionRejected},
		},
	})
	if err != nil {
		t.Fatalf("reject all plan leaves: %v", err)
	}
	if decisions.Plan.Revision != 2 {
		t.Fatalf("rejected plan revision = %d, want 2", decisions.Plan.Revision)
	}
	freezeRejected := FreezeCodingPlanCommand{
		OperationID: codingPlanOperationID(t, "rejected-freeze", job.ID),
		JobID:       job.ID, Generation: 1, Revision: 2,
		WorkspaceRoot: workspaceRoot, WorkspaceIdentity: workspaceIdentity,
	}
	if _, err := repository.FreezeCodingPlan(ctx, freezeRejected); !errors.Is(err, ErrCodingPlanState) {
		t.Fatalf("all-rejected freeze error = %v, want %v", err, ErrCodingPlanState)
	}
}

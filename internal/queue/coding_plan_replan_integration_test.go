package queue

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

func TestCodingPlanSameJobReplanSupersedesAndCarriesOnlyExactLeafIdentities(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	originalApproved := codingPlanExecutableLeaf(
		t, "A user can confirm the item.", model.CodingPlanDecisionPending, 1,
	)
	originalRejected := codingPlanExecutableLeaf(
		t, "A confirmation also sends a notification.", model.CodingPlanDecisionPending, 1,
	)
	originalPending := codingPlanExecutableLeaf(
		t, "A confirmation marker uses a distinct visual treatment.", model.CodingPlanDecisionPending, 1,
	)
	job, _, _ := storeCodingPlanFixture(
		t, repository, []CodingPlanLeafWrite{originalApproved, originalRejected, originalPending},
	)
	workspaceRoot, workspaceIdentity := codingPlanWorkspaceAuthority(t, job)
	if _, err := repository.ApplyCodingPlanDecisions(ctx, ApplyCodingPlanDecisionsCommand{
		OperationID: codingPlanOperationID(t, "generation-one-decisions", job.ID),
		JobID:       job.ID, Generation: 1, Revision: 1,
		WorkspaceRoot: workspaceRoot, WorkspaceIdentity: workspaceIdentity,
		Decisions: []CodingPlanDecisionChange{
			{LeafID: originalApproved.Leaf.ID, Decision: model.CodingPlanDecisionApproved},
			{LeafID: originalRejected.Leaf.ID, Decision: model.CodingPlanDecisionRejected},
		},
	}); err != nil {
		t.Fatalf("persist generation-one decisions: %v", err)
	}

	replanned, err := repository.ReplanJob(ctx, ReplanJobCommand{
		OperationID:   codingPlanOperationID(t, "replan", job.ID),
		JobID:         job.ID,
		Feedback:      "Keep confirmation, remove notification, and show a confirmation marker.",
		WorkspaceRoot: workspaceRoot, WorkspaceIdentity: workspaceIdentity,
	})
	if err != nil {
		t.Fatalf("replan same coding job: %v", err)
	}
	if !replanned.Applied || replanned.Job.ID != job.ID || replanned.Job.CurrentGeneration != 2 {
		t.Fatalf("replanned job = %#v, want same job generation 2", replanned)
	}
	var oldState model.CodingPlanState
	var oldRevision int64
	if err := pool.QueryRow(ctx, `
		SELECT state,revision FROM coding_plans WHERE job_id=$1 AND generation=1
	`, job.ID).Scan(&oldState, &oldRevision); err != nil {
		t.Fatalf("read superseded generation-one plan: %v", err)
	}
	if oldState != model.CodingPlanStateSuperseded || oldRevision != 3 {
		t.Fatalf("generation-one plan state/revision = %s/%d, want superseded/3", oldState, oldRevision)
	}

	prior, err := repository.PriorCodingPlanDecisions(ctx, job.ID, 2)
	if err != nil {
		t.Fatalf("load prior exact plan decisions: %v", err)
	}
	if prior[originalApproved.Leaf.ID].Decision != model.CodingPlanDecisionApproved ||
		prior[originalApproved.Leaf.ID].OriginGeneration != 1 ||
		prior[originalRejected.Leaf.ID].Decision != model.CodingPlanDecisionRejected ||
		prior[originalRejected.Leaf.ID].OriginGeneration != 1 {
		t.Fatalf("prior exact decisions = %#v", prior)
	}
	if _, exists := prior[originalPending.Leaf.ID]; exists {
		t.Fatalf("undecided leaf %q was incorrectly carried as a user decision", originalPending.Leaf.ID)
	}

	changed := codingPlanExecutableLeaf(
		t, "The confirmed item displays a confirmation marker.", model.CodingPlanDecisionPending, 2,
	)
	if _, exists := prior[changed.Leaf.ID]; exists {
		t.Fatalf("changed leaf identity %q inherited an unrelated decision", changed.Leaf.ID)
	}
	carried := codingPlanExecutableLeaf(
		t, originalApproved.Leaf.Statement, prior[originalApproved.Leaf.ID].Decision,
		prior[originalApproved.Leaf.ID].OriginGeneration,
	)
	pendingAgain := codingPlanExecutableLeaf(
		t, originalPending.Leaf.Statement, model.CodingPlanDecisionPending, 2,
	)
	claim, err := repository.ClaimNextStep(ctx, "coding-plan-generation-two-fixture")
	if err != nil {
		t.Fatalf("claim replanned review: %v", err)
	}
	if claim == nil || claim.Job.ID != job.ID || claim.Job.CurrentGeneration != 2 ||
		claim.Step.Action != "v3_coding_plan" {
		t.Fatalf("replanned review claim = %#v", claim)
	}
	plan, err := repository.StoreCodingPlanReview(ctx, StoreCodingPlanReviewCommand{
		Authority: claim.Authority, ScopeMode: model.CodingScopeModeNormal,
		RequestSHA256: assemblyline.ExactObjectiveContextSHA(job.Instruction + "\n" + "generation 2"),
		Leaves:        []CodingPlanLeafWrite{carried, changed, pendingAgain},
	})
	if err != nil {
		t.Fatalf("store generation-two review: %v", err)
	}
	if plan.JobID != job.ID || plan.Generation != 2 ||
		plan.Leaves[0].ID != originalApproved.Leaf.ID ||
		plan.Leaves[0].Decision != model.CodingPlanDecisionApproved ||
		plan.Leaves[1].ID != changed.Leaf.ID ||
		plan.Leaves[1].Decision != model.CodingPlanDecisionPending ||
		plan.Leaves[2].ID != pendingAgain.Leaf.ID ||
		plan.Leaves[2].Decision != model.CodingPlanDecisionPending {
		t.Fatalf("generation-two plan = %#v", plan)
	}
	var carriedOrigin, changedOrigin, pendingOrigin int64
	if err := pool.QueryRow(ctx, `
		SELECT
		  MAX(decision_origin_generation) FILTER (WHERE leaf_id=$3),
		  MAX(decision_origin_generation) FILTER (WHERE leaf_id=$4),
		  MAX(decision_origin_generation) FILTER (WHERE leaf_id=$5)
		FROM coding_plan_leaves WHERE job_id=$1 AND generation=$2
	`, job.ID, int64(2), carried.Leaf.ID, changed.Leaf.ID, pendingAgain.Leaf.ID).Scan(
		&carriedOrigin, &changedOrigin, &pendingOrigin,
	); err != nil {
		t.Fatalf("read generation-two decision origins: %v", err)
	}
	if carriedOrigin != 1 || changedOrigin != 2 || pendingOrigin != 2 {
		t.Fatalf(
			"decision origins = carried %d changed %d pending %d, want 1, 2, and 2",
			carriedOrigin, changedOrigin, pendingOrigin,
		)
	}
}

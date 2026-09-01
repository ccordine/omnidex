package queue

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

func TestCodingPlanStoreAndDatabaseRequireExactCarriedDecision(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	original := codingPlanExecutableLeaf(
		t, "A user can confirm the item.", model.CodingPlanDecisionPending, 1,
	)
	job, _, _ := storeCodingPlanFixture(t, repository, []CodingPlanLeafWrite{original})
	workspaceRoot, workspaceIdentity, err := codingJobWorkspaceBinding(job)
	if err != nil {
		t.Fatalf("read coding-plan workspace authority: %v", err)
	}
	if _, err := repository.ApplyCodingPlanDecisions(ctx, ApplyCodingPlanDecisionsCommand{
		OperationID: codingPlanOperationID(t, "carry-source", job.ID),
		JobID:       job.ID, Generation: 1, Revision: 1,
		WorkspaceRoot: workspaceRoot, WorkspaceIdentity: workspaceIdentity,
		Decisions: []CodingPlanDecisionChange{{
			LeafID: original.Leaf.ID, Decision: model.CodingPlanDecisionApproved,
		}},
	}); err != nil {
		t.Fatalf("persist prior decision: %v", err)
	}
	if _, err := repository.ReplanJob(ctx, ReplanJobCommand{
		OperationID: codingPlanOperationID(t, "carry-replan", job.ID),
		JobID:       job.ID, Feedback: "Keep confirmation and add a visible marker.",
		WorkspaceRoot: workspaceRoot, WorkspaceIdentity: workspaceIdentity,
	}); err != nil {
		t.Fatalf("create review generation for carry test: %v", err)
	}
	claim, err := repository.ClaimNextStep(ctx, "coding-plan-carry-integrity")
	if err != nil {
		t.Fatalf("claim carried-plan review: %v", err)
	}
	if claim == nil || claim.Job.ID != job.ID || claim.Job.CurrentGeneration != 2 ||
		claim.Step.Action != "v3_coding_plan" {
		t.Fatalf("carried-plan claim = %#v", claim)
	}

	newLeaf := codingPlanExecutableLeaf(
		t, "The confirmed item displays a visible marker.", model.CodingPlanDecisionPending, 2,
	)
	falseDecision := codingPlanExecutableLeaf(
		t, newLeaf.Leaf.Statement, model.CodingPlanDecisionApproved, 1,
	)
	if _, err := repository.StoreCodingPlanReview(ctx, StoreCodingPlanReviewCommand{
		Authority: claim.Authority, ScopeMode: model.CodingScopeModeNormal,
		RequestSHA256: assemblyline.ExactObjectiveContextSHA("false carried decision"),
		Leaves:        []CodingPlanLeafWrite{falseDecision},
	}); err == nil || !strings.Contains(err.Error(), "no exact prior user decision") {
		t.Fatalf("false carried decision error = %v", err)
	}

	suppressedPrior := codingPlanExecutableLeaf(
		t, original.Leaf.Statement, model.CodingPlanDecisionPending, 2,
	)
	if _, err := repository.StoreCodingPlanReview(ctx, StoreCodingPlanReviewCommand{
		Authority: claim.Authority, ScopeMode: model.CodingScopeModeNormal,
		RequestSHA256: assemblyline.ExactObjectiveContextSHA("suppressed carried decision"),
		Leaves:        []CodingPlanLeafWrite{suppressedPrior},
	}); err == nil || !strings.Contains(err.Error(), "differs from its exact prior user decision") {
		t.Fatalf("suppressed carried decision error = %v", err)
	}

	carried := codingPlanExecutableLeaf(
		t, original.Leaf.Statement, model.CodingPlanDecisionApproved, 1,
	)
	plan, err := repository.StoreCodingPlanReview(ctx, StoreCodingPlanReviewCommand{
		Authority: claim.Authority, ScopeMode: model.CodingScopeModeNormal,
		RequestSHA256: assemblyline.ExactObjectiveContextSHA("valid carried decision"),
		Leaves:        []CodingPlanLeafWrite{carried, newLeaf},
	})
	if err != nil {
		t.Fatalf("store exact carried decision: %v", err)
	}
	if plan.Leaves[0].Decision != model.CodingPlanDecisionApproved ||
		plan.Leaves[1].Decision != model.CodingPlanDecisionPending {
		t.Fatalf("stored carried plan = %#v", plan)
	}

	direct := codingPlanExecutableLeaf(
		t, "A separate report is generated.", model.CodingPlanDecisionApproved, 2,
	)
	if _, err := pool.Exec(ctx, `
		INSERT INTO coding_plan_leaves (
			job_id,generation,leaf_id,sort_index,statement,annotation,decision,
			decision_origin_generation,result_schema,candidate_sha256,
			kind_receipt_sha256,cardinality_receipt_sha256,result_relation
		) VALUES ($1,2,$2,2,$3,$4,$5,2,$6,$7,$8,$9,$10)
	`, job.ID, direct.Leaf.ID, direct.Leaf.Statement, direct.Leaf.Annotation,
		direct.Leaf.Decision, direct.ResultRelation.Schema,
		direct.ResultRelation.CandidateSHA256, direct.ResultRelation.KindReceiptSHA256,
		direct.ResultRelation.CardinalityReceiptSHA256, direct.ResultRelation.Relation,
	); err == nil || !strings.Contains(err.Error(), "active initial StoreCodingPlanReview transaction") {
		t.Fatalf("direct preapproved insert error = %v", err)
	}
	pendingAppend := codingPlanExecutableLeaf(
		t, "A pending leaf is appended after review persistence.", model.CodingPlanDecisionPending, 2,
	)
	if _, err := pool.Exec(ctx, `
		INSERT INTO coding_plan_leaves (
			job_id,generation,leaf_id,sort_index,statement,annotation,decision,
			decision_origin_generation,result_schema,candidate_sha256,
			kind_receipt_sha256,cardinality_receipt_sha256,result_relation
		) VALUES ($1,2,$2,2,$3,$4,$5,2,$6,$7,$8,$9,$10)
	`, job.ID, pendingAppend.Leaf.ID, pendingAppend.Leaf.Statement, pendingAppend.Leaf.Annotation,
		pendingAppend.Leaf.Decision, pendingAppend.ResultRelation.Schema,
		pendingAppend.ResultRelation.CandidateSHA256, pendingAppend.ResultRelation.KindReceiptSHA256,
		pendingAppend.ResultRelation.CardinalityReceiptSHA256, pendingAppend.ResultRelation.Relation,
	); err == nil || !strings.Contains(err.Error(), "active initial StoreCodingPlanReview transaction") {
		t.Fatalf("post-store pending append error = %v", err)
	}
}

func TestCodingPlanDecisionAndFreezeRequireLifecycleResultProvenance(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	leaf := codingPlanExecutableLeaf(
		t, "A user can archive one item.", model.CodingPlanDecisionPending, 1,
	)
	job, claim, _ := storeCodingPlanFixture(t, repository, []CodingPlanLeafWrite{leaf})
	workspaceRoot, workspaceIdentity, err := codingJobWorkspaceBinding(job)
	if err != nil {
		t.Fatalf("read coding-plan workspace authority: %v", err)
	}
	directInitial := codingPlanExecutableLeaf(
		t, "A report is generated without user approval.", model.CodingPlanDecisionApproved, 1,
	)
	if _, err := pool.Exec(ctx, `
		INSERT INTO coding_plan_leaves (
			job_id,generation,leaf_id,sort_index,statement,annotation,decision,
			decision_origin_generation,result_schema,candidate_sha256,
			kind_receipt_sha256,cardinality_receipt_sha256,result_relation
		) VALUES ($1,1,$2,1,$3,$4,$5,1,$6,$7,$8,$9,$10)
	`, job.ID, directInitial.Leaf.ID, directInitial.Leaf.Statement, directInitial.Leaf.Annotation,
		directInitial.Leaf.Decision, directInitial.ResultRelation.Schema,
		directInitial.ResultRelation.CandidateSHA256, directInitial.ResultRelation.KindReceiptSHA256,
		directInitial.ResultRelation.CardinalityReceiptSHA256, directInitial.ResultRelation.Relation,
	); err == nil || !strings.Contains(err.Error(), "active initial StoreCodingPlanReview transaction") {
		t.Fatalf("generation-one direct preapproval error = %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin direct decision mutation: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE coding_plan_leaves
		SET decision='approved',decision_origin_generation=1,updated_at=clock_timestamp()
		WHERE job_id=$1 AND generation=1 AND leaf_id=$2
	`, job.ID, leaf.Leaf.ID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("stage direct decision mutation: %v", err)
	}
	if err := tx.Commit(ctx); err == nil ||
		!strings.Contains(err.Error(), "leaf decision requires its exact lifecycle operation result") {
		t.Fatalf("direct decision commit error = %v", err)
	}
	current, err := repository.CurrentCodingPlan(ctx, job.ID)
	if err != nil {
		t.Fatalf("read plan after rejected direct decision: %v", err)
	}
	if current.Revision != 1 || current.Leaves[0].Decision != model.CodingPlanDecisionPending {
		t.Fatalf("direct decision mutation escaped rollback: %#v", current)
	}

	decided, err := repository.ApplyCodingPlanDecisions(ctx, ApplyCodingPlanDecisionsCommand{
		OperationID: codingPlanOperationID(t, "authoritative-before-direct-freeze", job.ID),
		JobID:       job.ID, Generation: 1, Revision: 1,
		WorkspaceRoot: workspaceRoot, WorkspaceIdentity: workspaceIdentity,
		Decisions: []CodingPlanDecisionChange{{
			LeafID: leaf.Leaf.ID, Decision: model.CodingPlanDecisionApproved,
		}},
	})
	if err != nil {
		t.Fatalf("apply authoritative decision: %v", err)
	}
	if decided.Plan.Revision != 2 {
		t.Fatalf("authoritative decision revision = %d, want 2", decided.Plan.Revision)
	}
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin mismatched direct decision mutation: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE coding_plan_leaves
		SET decision='rejected',decision_origin_generation=1,updated_at=clock_timestamp()
		WHERE job_id=$1 AND generation=1 AND leaf_id=$2
	`, job.ID, leaf.Leaf.ID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("stage mismatched direct decision mutation: %v", err)
	}
	if err := tx.Commit(ctx); err == nil ||
		!strings.Contains(err.Error(), "leaf decision requires its exact lifecycle operation result") {
		t.Fatalf("mismatched direct decision commit error = %v", err)
	}
	current, err = repository.CurrentCodingPlan(ctx, job.ID)
	if err != nil {
		t.Fatalf("read plan after mismatched direct decision: %v", err)
	}
	if current.Revision != 2 || current.Leaves[0].Decision != model.CodingPlanDecisionApproved {
		t.Fatalf("mismatched direct decision escaped rollback: %#v", current)
	}

	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin direct freeze mutation: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE coding_plans
		SET state='frozen',revision=3,updated_at=clock_timestamp(),frozen_at=clock_timestamp()
		WHERE job_id=$1 AND generation=1
	`, job.ID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("stage direct plan freeze: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status='completed',output='coding plan approved and frozen',
		    finished_at=clock_timestamp(),updated_at=clock_timestamp()
		WHERE id=$1 AND job_id=$2 AND generation=1
	`, claim.Step.ID, job.ID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("stage direct plan-step completion: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE jobs SET status='running',updated_at=clock_timestamp() WHERE id=$1
	`, job.ID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("stage direct plan job transition: %v", err)
	}
	if err := tx.Commit(ctx); err == nil ||
		!strings.Contains(err.Error(), "plan update requires its exact lifecycle operation result") {
		t.Fatalf("direct freeze commit error = %v", err)
	}
	current, err = repository.CurrentCodingPlan(ctx, job.ID)
	if err != nil {
		t.Fatalf("read plan after rejected direct freeze: %v", err)
	}
	if current.State != model.CodingPlanStateReview || current.Revision != 2 {
		t.Fatalf("direct freeze escaped rollback: %#v", current)
	}

	frozen, err := repository.FreezeCodingPlan(ctx, FreezeCodingPlanCommand{
		OperationID: codingPlanOperationID(t, "authoritative-freeze", job.ID),
		JobID:       job.ID, Generation: 1, Revision: 2,
		WorkspaceRoot: workspaceRoot, WorkspaceIdentity: workspaceIdentity,
	})
	if err != nil {
		t.Fatalf("authoritative freeze after rejected direct mutation: %v", err)
	}
	if frozen.Plan.State != model.CodingPlanStateFrozen || frozen.Plan.Revision != 3 {
		t.Fatalf("authoritative frozen plan = %#v", frozen.Plan)
	}
}

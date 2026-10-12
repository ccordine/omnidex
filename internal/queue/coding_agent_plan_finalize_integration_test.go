package queue

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestCodingAgentPlanningFreezesAndReplaysAtomically(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	repository.WithCodingAgents(codingAgentTestConfiguration())
	ctx := context.Background()
	job, err := repository.EnqueueCodingJobMode(ctx, "Build the approved feature.", t.TempDir(), codingagent.Agents)
	if err != nil {
		t.Fatal(err)
	}
	planner, err := repository.ClaimNextStep(ctx, "atomic-agent-plan-fixture")
	if err != nil || planner == nil {
		t.Fatalf("planning claim: %v %v", planner, err)
	}
	leaf := codingPlanExecutableLeaf(t, "The approved feature works.", model.CodingPlanDecisionPending, 1)
	command := StoreCodingPlanReviewCommand{Authority: planner.Authority, Leaves: []CodingPlanLeafWrite{leaf}}
	frozen, err := repository.StoreCodingPlanReview(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if frozen.State != model.CodingPlanStateFrozen || frozen.Leaves[0].Decision != model.CodingPlanDecisionApproved {
		t.Fatalf("not atomically finalized: %#v", frozen)
	}
	details, err := repository.CurrentJobDetails(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if details.Job.Status != model.JobStatusRunning || details.Steps[0].Status != model.StepStatusCompleted {
		t.Fatalf("planning left a waiting window: %#v", details)
	}
	replay, err := repository.StoreCodingPlanReview(ctx, command)
	if err != nil || !sameCodingPlanProjection(frozen, replay) {
		t.Fatalf("exact atomic replay: %#v %v", replay, err)
	}
	command.Leaves[0].Leaf.Statement = "A changed planning result."
	if _, err := repository.StoreCodingPlanReview(ctx, command); err == nil {
		t.Fatal("completed planning accepted changed input")
	}
	executor, err := repository.ClaimNextStep(ctx, "atomic-agent-execution-fixture")
	if err != nil || executor == nil || executor.Step.Action != "v3_coding" {
		t.Fatalf("execution did not follow finalized plan: %v %v", executor, err)
	}
	if _, err := repository.LoadFrozenCodingPlan(ctx, executor.Authority); err != nil {
		t.Fatal(err)
	}
}

func TestCodingAgentPlanningPreservesCarriedRejectedDecisions(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	kept := codingPlanExecutableLeaf(t, "Preserve the approved interaction.", model.CodingPlanDecisionPending, 1)
	rejected := codingPlanExecutableLeaf(t, "Preserve the rejected interaction.", model.CodingPlanDecisionPending, 1)
	job, _, plan := storeCodingPlanFixture(t, repository, []CodingPlanLeafWrite{kept, rejected})
	root, identity := codingPlanWorkspaceAuthority(t, job)
	decided, err := repository.ApplyCodingPlanDecisions(ctx, ApplyCodingPlanDecisionsCommand{OperationID: codingPlanOperationID(t, "historical-owner-decisions", job.ID), JobID: job.ID, Generation: 1, Revision: plan.Revision, WorkspaceRoot: root, WorkspaceIdentity: identity, Decisions: []CodingPlanDecisionChange{{LeafID: kept.Leaf.ID, Decision: model.CodingPlanDecisionApproved}, {LeafID: rejected.Leaf.ID, Decision: model.CodingPlanDecisionRejected}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.FreezeCodingPlan(ctx, FreezeCodingPlanCommand{OperationID: codingPlanOperationID(t, "historical-freeze", job.ID), JobID: job.ID, Generation: 1, Revision: decided.Plan.Revision, WorkspaceRoot: root, WorkspaceIdentity: identity}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ReplanJob(ctx, ReplanJobCommand{OperationID: codingPlanOperationID(t, "new-agent-generation", job.ID), JobID: job.ID, Feedback: "Continue while preserving exact owner decisions.", WorkspaceRoot: root, WorkspaceIdentity: identity}); err != nil {
		t.Fatal(err)
	}
	// This isolated fixture represents an existing job with retained owner decisions
	// and explicit Agents delegation. It does not add a product mode-change API.
	delegation, err := json.Marshal(codingAgentTestConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET metadata=jsonb_set(metadata,'{coding_execution}',$2::jsonb) WHERE id=$1`, job.ID, string(delegation)); err != nil {
		t.Fatal(err)
	}
	next, err := repository.ClaimNextStep(ctx, "carried-agent-plan-fixture")
	if err != nil || next == nil {
		t.Fatalf("replan claim: %v %v", next, err)
	}
	kept.Leaf.Decision = model.CodingPlanDecisionApproved
	rejected.Leaf.Decision = model.CodingPlanDecisionRejected
	added := codingPlanExecutableLeaf(t, "A newly delegated interaction works.", model.CodingPlanDecisionPending, 2)
	frozen, err := repository.StoreCodingPlanReview(ctx, StoreCodingPlanReviewCommand{Authority: next.Authority, Leaves: []CodingPlanLeafWrite{kept, rejected, added}})
	if err != nil {
		t.Fatal(err)
	}
	if frozen.State != model.CodingPlanStateFrozen || frozen.Leaves[0].Decision != model.CodingPlanDecisionApproved || frozen.Leaves[1].Decision != model.CodingPlanDecisionRejected || frozen.Leaves[2].Decision != model.CodingPlanDecisionApproved {
		t.Fatalf("delegated freeze changed carried owner decisions: %#v", frozen)
	}
}

package queue

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestCodingAgentPlanningWaitsForExplicitApprovalBeforeExecution(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	repository.WithCodingAgents(codingAgentTestConfiguration())
	ctx := context.Background()
	job, err := repository.EnqueueCodingJobMode(ctx, "Build the approved feature.", t.TempDir(), codingagent.Agents)
	if err != nil {
		t.Fatal(err)
	}
	planner, err := repository.ClaimNextStep(ctx, "approval-gated-agent-plan-fixture")
	if err != nil || planner == nil {
		t.Fatalf("planning claim: %v %v", planner, err)
	}
	command, _ := codingAgentPlanReviewFixture(t, repository, planner, "The approved feature works.")
	plan, err := repository.StoreCodingPlanReview(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if plan.State != model.CodingPlanStateReview || plan.Leaves[0].Decision != model.CodingPlanDecisionPending {
		t.Fatalf("proposal bypassed user review: %#v", plan)
	}
	details, err := repository.CurrentJobDetails(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if details.Job.Status != model.JobStatusWaiting || details.Steps[0].Status != model.StepStatusWaiting {
		t.Fatalf("planning did not pause for approval: %#v", details)
	}
	if execution, err := repository.ClaimNextStep(ctx, "premature-agent-execution-fixture"); err != nil || execution != nil {
		t.Fatalf("execution was claimable before approval: %v %v", execution, err)
	}
	replay, err := repository.StoreCodingPlanReview(ctx, command)
	if err != nil || !sameCodingPlanProjection(plan, replay) {
		t.Fatalf("exact proposal replay: %#v %v", replay, err)
	}
	changed := command
	changed.AgentWorkload = &codingagent.Workload{
		Tasks:  append([]codingagent.Task(nil), command.AgentWorkload.Tasks...),
		Checks: append([]codingagent.Check(nil), command.AgentWorkload.Checks...),
	}
	changed.AgentWorkload.Tasks[0].Goal = "A changed planning result."
	if _, err := repository.StoreCodingPlanReview(ctx, changed); err == nil {
		t.Fatal("stored planning accepted a changed workload")
	}
	frozen := freezeCodingAgentPlanFixture(t, repository, job, command)
	if frozen.State != model.CodingPlanStateFrozen || frozen.Leaves[0].Decision != model.CodingPlanDecisionApproved {
		t.Fatalf("explicitly approved plan was not frozen: %#v", frozen)
	}
	executor, err := repository.ClaimNextStep(ctx, "approved-agent-execution-fixture")
	if err != nil || executor == nil || executor.Step.Action != "v3_coding" {
		t.Fatalf("execution did not follow approved plan: %v %v", executor, err)
	}
	workload, err := repository.CodingAgentFrozenWorkload(ctx, executor.Authority)
	if err != nil {
		t.Fatal(err)
	}
	if len(workload.Tasks) != 1 || workload.Tasks[0].Goal != command.AgentWorkload.Tasks[0].Goal {
		t.Fatalf("execution workload differs from approved proposal: %#v", workload)
	}
}

func TestCodingAgentPlanningRejectsPartialWorkloadApproval(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	repository.WithCodingAgents(codingAgentTestConfiguration())
	ctx := context.Background()
	job, err := repository.EnqueueCodingJobMode(ctx, "Build both approved features.", t.TempDir(), codingagent.Agents)
	if err != nil {
		t.Fatal(err)
	}
	planner, err := repository.ClaimNextStep(ctx, "complete-agent-plan-fixture")
	if err != nil || planner == nil {
		t.Fatalf("planning claim: %v %v", planner, err)
	}
	workload := &codingagent.Workload{
		Tasks: []codingagent.Task{
			{ID: "one", Goal: "Build feature one.", Acceptance: []string{"Feature one works."}, WriteFiles: []string{"one.txt"}, RequirementIDs: []string{"objective"}, Checks: []codingagent.Check{{ID: "one-check", Argv: []string{"true"}}}},
			{ID: "two", Goal: "Build feature two.", Acceptance: []string{"Feature two works."}, WriteFiles: []string{"two.txt"}, RequirementIDs: []string{"objective"}, Checks: []codingagent.Check{{ID: "two-check", Argv: []string{"true"}}}},
		},
		Checks: []codingagent.Check{{ID: "overall-check", Argv: []string{"true"}}},
	}
	leaves := make([]CodingPlanLeafWrite, 0, len(workload.Tasks))
	for _, task := range workload.Tasks {
		statement, statementErr := codingagent.PlanTaskStatement(task)
		if statementErr != nil {
			t.Fatal(statementErr)
		}
		leaves = append(leaves, codingAgentPlanLeaf(t, statement, model.CodingPlanDecisionPending, planner.Authority.Generation))
	}
	if _, err := repository.CodingAgentEvidence(planner.Authority).Append(ctx, codingagent.Entry{Generation: planner.Authority.Generation, Phase: "plan_proposed", CallID: 1, Workload: workload}); err != nil {
		t.Fatal(err)
	}
	command := StoreCodingPlanReviewCommand{
		Authority:     planner.Authority,
		Leaves:        leaves,
		IntakeContext: &codingagent.IntakeContext{RedactedRequest: "Build both approved features."},
		AgentWorkload: workload,
	}
	plan, err := repository.StoreCodingPlanReview(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	root, identity := codingPlanWorkspaceAuthority(t, job)
	decided, err := repository.ApplyCodingPlanDecisions(ctx, ApplyCodingPlanDecisionsCommand{
		OperationID: codingPlanOperationID(t, "partial-agent-approval", job.ID), JobID: job.ID,
		Generation: plan.Generation, Revision: plan.Revision, WorkspaceRoot: root, WorkspaceIdentity: identity,
		Decisions: []CodingPlanDecisionChange{
			{LeafID: plan.Leaves[0].ID, Decision: model.CodingPlanDecisionApproved},
			{LeafID: plan.Leaves[1].ID, Decision: model.CodingPlanDecisionRejected},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repository.FreezeCodingPlan(ctx, FreezeCodingPlanCommand{
		OperationID: codingPlanOperationID(t, "reject-partial-agent-plan", job.ID), JobID: job.ID,
		Generation: plan.Generation, Revision: decided.Plan.Revision, WorkspaceRoot: root, WorkspaceIdentity: identity,
	})
	if err == nil || !strings.Contains(err.Error(), "complete proposed workload") {
		t.Fatalf("partial Agents workload freeze error = %v", err)
	}
}

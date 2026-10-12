package queue

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestCodingAgentPlanStepPauseRemainsVisibleWithoutExecutionQuestion(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	repository.WithCodingAgents(codingAgentTestConfiguration())
	ctx := context.Background()
	command := channelSessionCodingCommand(t, repository)
	command.CodingMode = codingagent.Agents
	result, err := repository.SubmitChannelSessionTurn(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	objective, err := repository.ClaimNextStep(ctx, "plan-pause-objective-fixture")
	if err != nil || objective == nil {
		t.Fatalf("objective claim: %v", err)
	}
	if err := repository.CompleteStep(ctx, CompleteStepCommand{
		OperationID: codingPlanOperationID(t, "plan-pause-workspace", result.Job.ID),
		Authority:   objective.Authority, StepID: objective.Step.ID, Output: "workspace_mutation",
	}); err != nil {
		t.Fatal(err)
	}
	planner, err := repository.ClaimNextStep(ctx, "original-plan-pause-fixture")
	if err != nil || planner == nil || planner.Step.Action != "v3_coding_plan" {
		t.Fatalf("original plan claim: %v", err)
	}
	// Seed an original persisted-plan review pause. It owns no execution-agent
	// questions evidence and its original plan must remain readable.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	leaf := codingPlanExecutableLeaf(t, "The approved behavior remains visible.", model.CodingPlanDecisionPending, 1)
	if _, err := tx.Exec(ctx, `INSERT INTO coding_plans(job_id,generation,revision,state,plan_step_id) VALUES($1,$2,1,'review',$3)`, result.Job.ID, result.Job.CurrentGeneration, planner.Step.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO coding_plan_leaves(job_id,generation,leaf_id,sort_index,statement,decision,decision_origin_generation,result_schema,result_relation) VALUES($1,$2,$3,0,$4,$5,$6,$7,$8)`, result.Job.ID, result.Job.CurrentGeneration, leaf.Leaf.ID, leaf.Leaf.Statement, leaf.Leaf.Decision, leaf.DecisionOriginGeneration, leaf.ResultRelation.Schema, leaf.ResultRelation.Relation); err != nil {
		t.Fatal(err)
	}
	if err := terminalizeStepAttemptTx(ctx, tx, planner.Authority, model.StepAttemptCompleted); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE job_steps SET status='waiting_input',worker_id=NULL,error='Original planning input is required.' WHERE id=$1`, planner.Step.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE jobs SET status='waiting_input' WHERE id=$1`, result.Job.ID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	details, err := repository.CurrentJobDetails(ctx, result.Job.ID)
	if err != nil {
		t.Fatalf("original planning pause became a missing Agent question: %v", err)
	}
	assertOriginalPlanningPause(t, details, planner.Step.ID)
	snapshot, err := repository.ChannelSessionSnapshot(ctx, command.ChannelID, MaxChannelSessionMessages, command.WorkspaceIdentity)
	if err != nil || snapshot.ActiveJob == nil {
		t.Fatalf("original planning pause unavailable in session snapshot: %v", err)
	}
	assertOriginalPlanningPause(t, *snapshot.ActiveJob, planner.Step.ID)
	plan, err := repository.CurrentCodingPlan(ctx, result.Job.ID)
	if err != nil || plan.State != model.CodingPlanStateReview || len(plan.Leaves) != 1 || plan.Leaves[0] != leaf.Leaf {
		t.Fatalf("original plan review changed: error=%v", err)
	}
}

func assertOriginalPlanningPause(t *testing.T, details model.JobDetails, planStepID int64) {
	t.Helper()
	if details.Job.Status != model.JobStatusWaiting || details.AgentPause != nil {
		t.Fatal("original planning pause was presented as an execution-agent question")
	}
	for _, step := range details.Steps {
		if step.ID == planStepID && step.Action == "v3_coding_plan" && step.Status == model.StepStatusWaiting {
			return
		}
	}
	t.Fatal("original waiting plan step was not preserved")
}

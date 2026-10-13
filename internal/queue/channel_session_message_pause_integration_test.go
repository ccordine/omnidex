package queue

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestActiveMessageIntakePreventsNativeQuestionPauseUntilResolved(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	planner, session, leaf := messagePausePlannerClaim(t, repository, codingagent.Agents)
	if _, err := repository.StoreCodingPlanReview(ctx, StoreCodingPlanReviewCommand{
		Authority: planner.Authority, Leaves: []CodingPlanLeafWrite{leaf},
	}); err != nil {
		t.Fatal(err)
	}
	executor, err := repository.ClaimNextStep(ctx, "message-question-pause-fixture")
	if err != nil || executor == nil || executor.Step.Action != "v3_coding" {
		t.Fatalf("claim execution step: %#v %v", executor, err)
	}
	questions := []string{"Choose an approved detail."}
	if _, err := repository.CodingAgentEvidence(executor.Authority).Append(ctx, codingagent.Entry{
		Generation: executor.Authority.Generation, Phase: "questions", Questions: questions,
	}); err != nil {
		t.Fatal(err)
	}
	// Force the dangerous ordering: intake commits after the worker has its
	// durable native question, before the same attempt tries to enter waiting.
	message := observedSessionQuestion(t, session, executor.Job.ID, executor.Authority.Generation)
	if _, err := repository.SubmitChannelSessionTurn(ctx, message); err != nil {
		t.Fatal(err)
	}
	if err := repository.PauseCodingAgentStep(ctx, executor.Authority, questions); !errors.Is(err, ErrPendingChannelSessionMessage) {
		t.Fatalf("native question pause stranded accepted input: %v", err)
	}
	assertMessagePauseAttemptStillActive(t, repository, executor, message.OperationID)
	resolveMessageBeforePause(t, repository, executor, message.OperationID)
	if err := repository.PauseCodingAgentStep(ctx, executor.Authority, questions); err != nil {
		t.Fatalf("original question pause did not resume after conversation resolution: %v", err)
	}
	pause, err := repository.ReadCodingAgentPause(ctx, executor.Job.ID, executor.Authority.Generation)
	if err != nil || pause.StepID != executor.Step.ID || pause.WaitingAttempt != executor.Authority.Attempt ||
		!slices.Equal(pause.Questions, questions) {
		t.Fatalf("retried pause changed its original question authority: %#v %v", pause, err)
	}
}

func TestActiveMessageIntakePreventsPlanReviewWaitUntilResolved(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	planner, session, leaf := messagePausePlannerClaim(t, repository, codingagent.Normal)
	command := StoreCodingPlanReviewCommand{Authority: planner.Authority, Leaves: []CodingPlanLeafWrite{leaf}}
	message := observedSessionQuestion(t, session, planner.Job.ID, planner.Authority.Generation)
	if _, err := repository.SubmitChannelSessionTurn(ctx, message); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.StoreCodingPlanReview(ctx, command); !errors.Is(err, ErrPendingChannelSessionMessage) {
		t.Fatalf("plan review wait stranded accepted input: %v", err)
	}
	assertMessagePauseAttemptStillActive(t, repository, planner, message.OperationID)
	var planCount, leafCount int
	if err := pool.QueryRow(ctx, `SELECT
	 (SELECT count(*) FROM coding_plans WHERE job_id=$1),
	 (SELECT count(*) FROM coding_plan_leaves WHERE job_id=$1)`, planner.Job.ID).Scan(&planCount, &leafCount); err != nil {
		t.Fatal(err)
	}
	if planCount != 0 || leafCount != 0 {
		t.Fatalf("blocked plan store left partial authority: plans=%d leaves=%d", planCount, leafCount)
	}
	resolveMessageBeforePause(t, repository, planner, message.OperationID)
	plan, err := repository.StoreCodingPlanReview(ctx, command)
	if err != nil || plan.State != model.CodingPlanStateReview || len(plan.Leaves) != 1 || plan.Leaves[0] != leaf.Leaf {
		t.Fatalf("original plan command did not resume unchanged: %#v %v", plan, err)
	}
	details, err := repository.CurrentJobDetails(ctx, planner.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertOriginalPlanningPause(t, details, planner.Step.ID)
	replay, err := repository.StoreCodingPlanReview(ctx, command)
	if err != nil || replay.JobID != plan.JobID || replay.Generation != plan.Generation || replay.Revision != plan.Revision {
		t.Fatalf("successful plan wait lost historical replay: %#v %v", replay, err)
	}
}

func messagePausePlannerClaim(t *testing.T, repository *Repository, mode codingagent.Mode) (*model.ClaimedStep, ChannelSessionTurnCommand, CodingPlanLeafWrite) {
	t.Helper()
	ctx := context.Background()
	if mode == codingagent.Agents {
		repository.WithCodingAgents(codingAgentTestConfiguration())
	}
	command := channelSessionCodingCommand(t, repository)
	command.CodingMode = mode
	initial, err := repository.SubmitChannelSessionTurn(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	objective, err := repository.ClaimNextStep(ctx, "message-pause-objective-fixture")
	if err != nil || objective == nil || objective.Step.Action != "objective_resolve" {
		t.Fatalf("claim objective step: %#v %v", objective, err)
	}
	if err := repository.CompleteStep(ctx, CompleteStepCommand{
		OperationID: codingPlanOperationID(t, "message-pause-workspace", initial.Job.ID),
		Authority:   objective.Authority, StepID: objective.Step.ID, Output: "workspace_mutation",
	}); err != nil {
		t.Fatal(err)
	}
	planner, err := repository.ClaimNextStep(ctx, "message-pause-planner-fixture")
	if err != nil || planner == nil || planner.Step.Action != "v3_coding_plan" {
		t.Fatalf("claim plan step: %#v %v", planner, err)
	}
	leaf := codingPlanExecutableLeaf(t, "The approved interaction works.", model.CodingPlanDecisionPending, planner.Authority.Generation)
	return planner, command, leaf
}

func assertMessagePauseAttemptStillActive(t *testing.T, repository *Repository, claim *model.ClaimedStep, operation model.LifecycleOperationID) {
	t.Helper()
	ctx := context.Background()
	if err := repository.RequireActiveStepAttempt(ctx, claim.Authority); err != nil {
		t.Fatalf("blocked pause changed active attempt: %v", err)
	}
	message, err := repository.NextChannelSessionMessage(ctx, claim.Authority)
	if err != nil || message == nil || message.OperationID != operation {
		t.Fatalf("blocked pause lost its accepted message: %#v %v", message, err)
	}
	question, err := readChannelSessionMessageResolutionTxForPause(t, repository, operation)
	if err != nil || question {
		t.Fatalf("blocked pause falsely retired or answered the message: resolved=%t error=%v", question, err)
	}
}

func readChannelSessionMessageResolutionTxForPause(t *testing.T, repository *Repository, operation model.LifecycleOperationID) (bool, error) {
	t.Helper()
	ctx := context.Background()
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	_, found, err := readChannelSessionMessageResolutionTx(ctx, tx, operation)
	return found, err
}

func resolveMessageBeforePause(t *testing.T, repository *Repository, claim *model.ClaimedStep, operation model.LifecycleOperationID) {
	t.Helper()
	result, err := repository.ResolveChannelSessionMessage(context.Background(), ResolveChannelSessionMessageCommand{
		Authority: claim.Authority, OperationID: operation,
		Kind: assemblyline.ObjectiveKindAnswer, Response: "The current task is applying the approved interaction.",
	})
	if err != nil || !result.Applied || result.Replanned || result.Job.ID != claim.Job.ID || result.Job.CurrentGeneration != claim.Authority.Generation {
		t.Fatalf("conversation resolution changed pause authority: %#v %v", result, err)
	}
}

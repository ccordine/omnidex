package queue

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestCodingAgentFeedbackBindsObservedGenerationAndReplaysAfterReplan(t *testing.T) {
	pool, repository, job, claim, _ := freshAgentCodingClaim(t)
	ctx := context.Background()
	questions := []string{"Choose the approved first-generation detail."}
	if _, err := repository.CodingAgentEvidence(claim.Authority).Append(ctx, codingagent.Entry{Generation: 1, Phase: "questions", Questions: questions}); err != nil {
		t.Fatal(err)
	}
	if err := repository.PauseCodingAgentStep(ctx, claim.Authority, questions); err != nil {
		t.Fatal(err)
	}
	root, identity := codingPlanWorkspaceAuthority(t, job)
	command := SubmitJobFeedbackCommand{OperationID: codingPlanOperationID(t, "generation-bound-answer", job.ID), JobID: job.ID, Feedback: "Use the approved detail.", WorkspaceRoot: root, WorkspaceIdentity: identity}
	if _, err := repository.SubmitJobFeedback(ctx, command); err == nil {
		t.Fatal("Agents accepted feedback without an observed generation")
	}
	first := int64(1)
	command.ExpectedGeneration = &first
	command.ExpectedWaitingAttempt = &claim.Authority.Attempt
	accepted, err := repository.SubmitJobFeedback(ctx, command)
	if err != nil || !accepted.Applied {
		t.Fatalf("current generation answer: %#v %v", accepted, err)
	}
	if _, err := repository.ReplanJob(ctx, ReplanJobCommand{OperationID: codingPlanOperationID(t, "later-generation", job.ID), JobID: job.ID, Feedback: "Implement the next approved behavior.", WorkspaceRoot: root, WorkspaceIdentity: identity}); err != nil {
		t.Fatal(err)
	}
	planner, err := repository.ClaimNextStep(ctx, "generation-two-plan-fixture")
	if err != nil || planner == nil {
		t.Fatalf("second planning: %v %v", planner, err)
	}
	leaf := codingPlanExecutableLeaf(t, "The second approved behavior works.", model.CodingPlanDecisionPending, 2)
	if _, err := repository.StoreCodingPlanReview(ctx, StoreCodingPlanReviewCommand{Authority: planner.Authority, Leaves: []CodingPlanLeafWrite{leaf}}); err != nil {
		t.Fatal(err)
	}
	executor, err := repository.ClaimNextStep(ctx, "generation-two-executor-fixture")
	if err != nil || executor == nil {
		t.Fatalf("second execution: %v %v", executor, err)
	}
	laterQuestions := []string{"Choose the approved second-generation detail."}
	if _, err := repository.CodingAgentEvidence(executor.Authority).Append(ctx, codingagent.Entry{Generation: 2, Phase: "questions", Questions: laterQuestions}); err != nil {
		t.Fatal(err)
	}
	if err := repository.PauseCodingAgentStep(ctx, executor.Authority, laterQuestions); err != nil {
		t.Fatal(err)
	}
	stale := command
	stale.OperationID = codingPlanOperationID(t, "stale-generation-answer", job.ID)
	if _, err := repository.SubmitJobFeedback(ctx, stale); !errors.Is(err, ErrJobFeedbackGeneration) {
		t.Fatalf("stale answer entered new generation: %v", err)
	}
	var secondAnswers int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM evidence WHERE job_id=$1 AND kind='coding_agent' AND payload_json->>'generation'='2' AND payload_json->>'phase'='answers'`, job.ID).Scan(&secondAnswers); err != nil || secondAnswers != 0 {
		t.Fatalf("stale answer changed durable state: %d %v", secondAnswers, err)
	}
	replay, err := repository.SubmitJobFeedback(ctx, command)
	if err != nil || replay.Applied || replay.Job.CurrentGeneration != 1 {
		t.Fatalf("exact historical generation replay: %#v %v", replay, err)
	}
	second := int64(2)
	command.ExpectedGeneration = &second
	if _, err := repository.SubmitJobFeedback(ctx, command); !errors.Is(err, ErrLifecycleOperationConflict) {
		t.Fatalf("old operation changed observed generation: %v", err)
	}
}

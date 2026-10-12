package queue

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
)

func TestCodingAgentChannelSessionFeedbackResumesAndReplays(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	repository.WithCodingAgents(codingAgentTestConfiguration())
	ctx := context.Background()
	root := t.TempDir()
	directory, err := projectroot.DirectoryIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := projectroot.ClientWorkspaceIdentity("11111111111111111111111111111111", directory)
	if err != nil {
		t.Fatal(err)
	}
	channel, err := repository.EnsureCLIChatSessionChannel(ctx, root, identity)
	if err != nil {
		t.Fatal(err)
	}
	_, job, err := repository.EnqueueChannelTurnWithCodingMode(ctx, channel.ID, "Build an approved interaction.", "", codingagent.Agents)
	if err != nil {
		t.Fatal(err)
	}
	objective, err := repository.ClaimNextStep(ctx, "agent-session-objective-fixture")
	if err != nil || objective == nil {
		t.Fatalf("objective: %v %v", objective, err)
	}
	if err := repository.CompleteStep(ctx, CompleteStepCommand{OperationID: codingPlanOperationID(t, "session-workspace", job.ID), Authority: objective.Authority, StepID: objective.Step.ID, Output: "workspace_mutation"}); err != nil {
		t.Fatal(err)
	}
	planner, err := repository.ClaimNextStep(ctx, "agent-session-planner-fixture")
	if err != nil || planner == nil {
		t.Fatalf("planner: %v %v", planner, err)
	}
	leaf := codingPlanExecutableLeaf(t, "The approved interaction works.", model.CodingPlanDecisionPending, 1)
	if _, err := repository.StoreCodingPlanReview(ctx, StoreCodingPlanReviewCommand{Authority: planner.Authority, Leaves: []CodingPlanLeafWrite{leaf}}); err != nil {
		t.Fatal(err)
	}
	executor, err := repository.ClaimNextStep(ctx, "agent-session-executor-fixture")
	if err != nil || executor == nil {
		t.Fatalf("executor: %v %v", executor, err)
	}
	questions := []string{"Choose an approved detail."}
	if _, err := repository.CodingAgentEvidence(executor.Authority).Append(ctx, codingagent.Entry{Generation: 1, Phase: "questions", Questions: questions}); err != nil {
		t.Fatal(err)
	}
	if err := repository.PauseCodingAgentStep(ctx, executor.Authority, questions); err != nil {
		t.Fatal(err)
	}
	command := ChannelSessionTurnCommand{OperationID: codingPlanOperationID(t, "session-answer", job.ID), ChannelID: channel.ID, WorkspaceRoot: root, WorkspaceIdentity: identity, Text: "Use the approved detail."}
	result, err := repository.SubmitChannelSessionTurn(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Applied || result.Disposition != model.ChannelSessionTurnFeedback || result.Job.ID != job.ID || result.Job.CurrentGeneration != 1 || result.Job.Status != model.JobStatusRunning {
		t.Fatalf("session answer changed job authority: %#v", result)
	}
	resumed, err := repository.ClaimNextStep(ctx, "agent-session-resume-fixture")
	if err != nil || resumed == nil || resumed.Authority.StepID != executor.Authority.StepID {
		t.Fatalf("resumed: %v %v", resumed, err)
	}
	more := []string{"Choose another approved detail."}
	if _, err := repository.CodingAgentEvidence(resumed.Authority).Append(ctx, codingagent.Entry{Generation: 1, Phase: "questions", Questions: more}); err != nil {
		t.Fatal(err)
	}
	if err := repository.PauseCodingAgentStep(ctx, resumed.Authority, more); err != nil {
		t.Fatal(err)
	}
	replay, err := repository.SubmitChannelSessionTurn(ctx, command)
	if err != nil || replay.Applied || replay.Job.ID != job.ID || replay.Job.Status != model.JobStatusRunning {
		t.Fatalf("historical session answer replay: %#v %v", replay, err)
	}
}

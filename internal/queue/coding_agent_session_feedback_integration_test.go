package queue

import (
	"context"
	"slices"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
)

func TestCodingAgentChannelSessionFeedbackResumesAndReplays(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	frozenConfiguration := codingAgentTestConfiguration()
	repository.WithCodingAgents(frozenConfiguration)
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
	planner, err := repository.ClaimNextStep(ctx, "agent-session-planner-fixture")
	if err != nil || planner == nil {
		t.Fatalf("planner: %v %v", planner, err)
	}
	planCommand := codingAgentPlanReviewFixture(t, repository, planner, "The approved interaction works.")
	freezeCodingAgentPlanFixture(t, repository, job, planCommand)
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
	snapshot, err := repository.ChannelSessionSnapshot(ctx, channel.ID, MaxChannelSessionMessages, identity)
	if err != nil || snapshot.ActiveJob == nil || snapshot.ActiveJob.AgentPause == nil ||
		!slices.Equal(snapshot.ActiveJob.AgentPause.Questions, questions) ||
		snapshot.ActiveJob.AgentPause.Generation != job.CurrentGeneration ||
		snapshot.ActiveJob.AgentPause.StepID != executor.Step.ID ||
		snapshot.ActiveJob.AgentPause.WaitingAttempt != executor.Authority.Attempt {
		t.Fatalf("session snapshot did not project exact durable question: error=%v", err)
	}
	details, err := repository.CurrentJobDetails(ctx, job.ID)
	if err != nil || details.AgentPause == nil || !slices.Equal(details.AgentPause.Questions, questions) {
		t.Fatalf("current job omitted durable question: pause=%#v error=%v", details.AgentPause, err)
	}
	changed := frozenConfiguration
	changed.Executor.Model = "later-executor"
	if err := repository.StoreCodingAgentConfiguration(ctx, changed); err != nil {
		t.Fatal(err)
	}
	command := ChannelSessionTurnCommand{OperationID: codingPlanOperationID(t, "session-answer", job.ID), ChannelID: channel.ID, WorkspaceRoot: root, WorkspaceIdentity: identity, Text: "Use the approved detail.", CodingMode: codingagent.Agents}
	result, err := repository.SubmitChannelSessionTurn(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Applied || result.Disposition != model.ChannelSessionTurnFeedback || result.Job.ID != job.ID || result.Job.CurrentGeneration != 1 || result.Job.Status != model.JobStatusRunning {
		t.Fatalf("session answer changed job authority: %#v", result)
	}
	assertSessionCodingConfiguration(t, result.Job, frozenConfiguration)
	snapshot, err = repository.ChannelSessionSnapshot(ctx, channel.ID, MaxChannelSessionMessages, identity)
	if err != nil || snapshot.ActiveJob == nil || snapshot.ActiveJob.AgentPause != nil {
		t.Fatalf("answered question remains projected: error=%v", err)
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
	assertSessionCodingConfiguration(t, replay.Job, frozenConfiguration)
	snapshot, err = repository.ChannelSessionSnapshot(ctx, channel.ID, MaxChannelSessionMessages, identity)
	if err != nil || snapshot.ActiveJob == nil || snapshot.ActiveJob.AgentPause == nil ||
		!slices.Equal(snapshot.ActiveJob.AgentPause.Questions, more) ||
		snapshot.ActiveJob.AgentPause.WaitingAttempt != resumed.Authority.Attempt {
		t.Fatalf("replay changed current question projection: error=%v", err)
	}
	command.OperationID = codingPlanOperationID(t, "answer-current-session-question", job.ID)
	command.Text = "Use the second approved detail."
	if _, err := repository.SubmitChannelSessionTurn(ctx, command); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.InterruptJob(ctx, ReplanJobCommand{
		OperationID: codingPlanOperationID(t, "interrupt-paused-session", job.ID),
		JobID:       job.ID, Feedback: "Pause for a deliberate redirect.",
		WorkspaceRoot: root, WorkspaceIdentity: identity,
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err = repository.ChannelSessionSnapshot(ctx, channel.ID, MaxChannelSessionMessages, identity)
	if err != nil || snapshot.ActiveJob == nil || snapshot.ActiveJob.AgentPause != nil ||
		snapshot.ActiveJob.Job.CurrentGeneration != 2 || snapshot.ActiveJob.Job.Status != model.JobStatusWaiting {
		t.Fatalf("interruption became an Agent question: error=%v", err)
	}
}

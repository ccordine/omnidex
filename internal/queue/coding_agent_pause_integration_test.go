package queue

import (
	"context"
	"slices"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestCodingAgentPlanningQuestionRemainsVisibleAndResumesSamePlan(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	repository.WithCodingAgents(codingAgentTestConfiguration())
	ctx := context.Background()
	command := channelSessionCodingCommand(t, repository)
	command.CodingMode = codingagent.Agents
	result, err := repository.SubmitChannelSessionTurn(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	objective, err := repository.ClaimNextStep(ctx, "plan-question-objective-fixture")
	if err != nil || objective == nil {
		t.Fatalf("objective claim: %v", err)
	}
	if err := repository.CompleteStep(ctx, CompleteStepCommand{
		OperationID: codingPlanOperationID(t, "plan-question-workspace", result.Job.ID),
		Authority:   objective.Authority, StepID: objective.Step.ID, Output: "workspace_mutation",
	}); err != nil {
		t.Fatal(err)
	}
	planner, err := repository.ClaimNextStep(ctx, "plan-question-fixture")
	if err != nil || planner == nil || planner.Step.Action != "v3_coding_plan" {
		t.Fatalf("planning claim: %#v %v", planner, err)
	}
	questions := []string{"Which explicitly supported output format should the plan implement?"}
	questionID, err := repository.CodingAgentEvidence(planner.Authority).Append(ctx, codingagent.Entry{
		Generation: planner.Authority.Generation, Phase: "questions", Questions: questions,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.PauseCodingAgentStep(ctx, planner.Authority, questions); err != nil {
		t.Fatal(err)
	}
	details, err := repository.CurrentJobDetails(ctx, result.Job.ID)
	if err != nil || details.AgentPause == nil || details.Job.Status != model.JobStatusWaiting ||
		details.AgentPause.StepID != planner.Step.ID || !slices.Equal(details.AgentPause.Questions, questions) {
		t.Fatalf("planning question was not projected: pause=%#v error=%v", details.AgentPause, err)
	}
	snapshot, err := repository.ChannelSessionSnapshot(ctx, command.ChannelID, MaxChannelSessionMessages, command.WorkspaceIdentity)
	if err != nil || snapshot.ActiveJob == nil || snapshot.ActiveJob.AgentPause == nil ||
		!slices.Equal(snapshot.ActiveJob.AgentPause.Questions, questions) {
		t.Fatalf("planning question unavailable in chat: snapshot=%#v error=%v", snapshot.ActiveJob, err)
	}
	answer := command
	answer.OperationID = codingPlanOperationID(t, "answer-planning-question", result.Job.ID)
	answer.Text = "Use JSON."
	resumed, err := repository.SubmitChannelSessionTurn(ctx, answer)
	if err != nil || !resumed.Applied || resumed.Job.Status != model.JobStatusRunning || resumed.Job.CurrentGeneration != planner.Authority.Generation {
		t.Fatalf("planning answer did not resume the same generation: %#v %v", resumed, err)
	}
	next, err := repository.ClaimNextStep(ctx, "resumed-plan-question-fixture")
	if err != nil || next == nil || next.Step.ID != planner.Step.ID || next.Authority.Attempt == planner.Authority.Attempt {
		t.Fatalf("planning answer did not resume the same step in a new attempt: %#v %v", next, err)
	}
	entries, err := repository.CodingAgentEvidence(next.Authority).Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range entries {
		found = found || entry.Phase == "answers" && entry.QuestionID == questionID && slices.Equal(entry.Answers, []string{"Use JSON."})
	}
	if !found {
		t.Fatal("resumed planning omitted the exact durable answer")
	}
}

package queue

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestChatObjectiveCompletionSelectsExactlyOnePersistedTailOutcome(t *testing.T) {
	t.Run("workspace mutation advances into plan review", func(t *testing.T) {
		_, repository := freshCodingPlanRepository(t)
		ctx := context.Background()
		channel, job := enqueueChatCodingPipelineFixture(t, repository, "Build the confirmation interaction.")
		claim, err := repository.ClaimNextStep(ctx, "chat-workspace-objective")
		if err != nil {
			t.Fatal(err)
		}
		requireChatObjectiveClaim(t, claim, job.ID, 1)
		if err := repository.CompleteStep(ctx, CompleteStepCommand{
			OperationID: codingPlanOperationID(t, "chat-workspace-handoff", job.ID),
			Authority:   claim.Authority,
			StepID:      claim.Step.ID,
			Output:      "workspace_mutation",
		}); err != nil {
			t.Fatal(err)
		}
		details, err := repository.CurrentJobDetails(ctx, job.ID)
		if err != nil {
			t.Fatal(err)
		}
		requireChatPipelineState(
			t, details, model.JobStatusRunning,
			model.StepStatusCompleted, model.StepStatusPending, model.StepStatusPending,
		)
		next, err := repository.ClaimNextStep(ctx, "chat-plan-after-objective")
		if err != nil {
			t.Fatal(err)
		}
		if next == nil || next.Job.ID != job.ID || next.Step.Action != "v3_coding_plan" {
			t.Fatalf("claim after workspace objective = %#v", next)
		}
		messages, err := repository.ListChannelMessages(ctx, channel.ID, 10, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(messages.Messages) != 1 || messages.Messages[0].Role != model.ChannelMessageRoleUser {
			t.Fatalf("workspace handoff materialized a premature assistant response: %#v", messages.Messages)
		}
	})

	t.Run("nonmutation cancels only coding tail and completes chat", func(t *testing.T) {
		_, repository := freshCodingPlanRepository(t)
		ctx := context.Background()
		channel, job := enqueueChatCodingPipelineFixture(t, repository, "Explain the confirmation state.")
		claim, err := repository.ClaimNextStep(ctx, "chat-answer-objective")
		if err != nil {
			t.Fatal(err)
		}
		requireChatObjectiveClaim(t, claim, job.ID, 1)
		command := CompleteStepEvidenceCommand{CompleteStepCommand: CompleteStepCommand{
			OperationID: codingPlanOperationID(t, "chat-answer-completion", job.ID),
			Authority:   claim.Authority,
			StepID:      claim.Step.ID,
			Output:      "The item is confirmed.",
			ContextKey:  "objective_result",
		}}
		if err := repository.CompleteStepWithEvidence(ctx, command); err != nil {
			t.Fatal(err)
		}
		details, err := repository.CurrentJobDetails(ctx, job.ID)
		if err != nil {
			t.Fatal(err)
		}
		requireChatPipelineState(
			t, details, model.JobStatusCompleted,
			model.StepStatusCompleted, model.StepStatusCanceled, model.StepStatusCanceled,
		)
		if details.Job.Result != "The item is confirmed." {
			t.Fatalf("completed chat result = %q", details.Job.Result)
		}
		if err := repository.CompleteStepWithEvidence(ctx, command); err != nil {
			t.Fatalf("replay exact terminal objective completion: %v", err)
		}
		if next, err := repository.ClaimNextStep(ctx, "chat-after-answer"); err != nil || next != nil {
			t.Fatalf("claim after terminal answer = %#v err=%v", next, err)
		}
		messages, err := repository.ListChannelMessages(ctx, channel.ID, 10, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(messages.Messages) != 2 ||
			messages.Messages[1].Role != model.ChannelMessageRoleAssistant ||
			messages.Messages[1].Content != "The item is confirmed." {
			t.Fatalf("terminal chat messages = %#v", messages.Messages)
		}
	})
}

func TestChatReplanRestartsAtObjectiveBoundaryWithFullCodingTail(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	_, job := enqueueChatCodingPipelineFixture(t, repository, "Build the confirmation interaction.")
	claim, err := repository.ClaimNextStep(ctx, "chat-replan-objective-one")
	if err != nil {
		t.Fatal(err)
	}
	requireChatObjectiveClaim(t, claim, job.ID, 1)
	if err := repository.CompleteStep(ctx, CompleteStepCommand{
		OperationID: codingPlanOperationID(t, "chat-replan-handoff", job.ID),
		Authority:   claim.Authority,
		StepID:      claim.Step.ID,
		Output:      "workspace_mutation",
	}); err != nil {
		t.Fatal(err)
	}
	const feedback = "Keep confirmation local and visible."
	result, err := repository.ReplanJob(ctx, ReplanJobCommand{
		OperationID: codingPlanOperationID(t, "chat-objective-replan", job.ID),
		JobID:       job.ID,
		Feedback:    feedback,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Applied || result.Job.ID != job.ID || result.Job.CurrentGeneration != 2 {
		t.Fatalf("same-job chat replan = %#v", result)
	}
	details, err := repository.CurrentJobDetails(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	requireChatPipelineState(
		t, details, model.JobStatusRunning,
		model.StepStatusPending, model.StepStatusPending, model.StepStatusPending,
	)
	if details.Job.CurrentGeneration != 2 {
		t.Fatalf("replanned generation = %d", details.Job.CurrentGeneration)
	}
	continuity, err := repository.ObjectiveContinuityAuthorities(
		ctx, details.Job, "objective_resolve",
	)
	if err != nil {
		t.Fatal(err)
	}
	if continuity.Replan == nil || continuity.Replan.JobID != job.ID ||
		continuity.Replan.Generation != 2 || continuity.Replan.Feedback != feedback {
		t.Fatalf("chat objective continuity = %#v", continuity)
	}
	claim, err = repository.ClaimNextStep(ctx, "chat-replan-objective-two")
	if err != nil {
		t.Fatal(err)
	}
	requireChatObjectiveClaim(t, claim, job.ID, 2)
}

func enqueueChatCodingPipelineFixture(
	t *testing.T,
	repository *Repository,
	instruction string,
) (model.Channel, model.Job) {
	t.Helper()
	ctx := context.Background()
	channelID := model.ChannelID("chat-plan-" + codingPlanTestNonce(t))
	channel, err := repository.CreateChannel(ctx, model.Channel{
		ID: channelID, Scope: model.ChannelScopeUser,
		Name: "Chat plan boundary fixture", Tags: []string{"chat"},
		WorkspaceRoot: t.TempDir(), Mode: model.ChannelModeAssistant,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, job, err := repository.EnqueueChannelTurn(ctx, channel.ID, instruction)
	if err != nil {
		t.Fatal(err)
	}
	return channel, job
}

func requireChatObjectiveClaim(
	t *testing.T,
	claim *model.ClaimedStep,
	jobID, generation int64,
) {
	t.Helper()
	if claim == nil || claim.Job.ID != jobID || claim.Job.Pipeline != model.PipelineChat ||
		claim.Job.CurrentGeneration != generation || claim.Step.Action != "objective_resolve" ||
		claim.Step.Generation != generation {
		t.Fatalf("chat objective claim = %#v", claim)
	}
}

func requireChatPipelineState(
	t *testing.T,
	details model.JobDetails,
	jobStatus, objectiveStatus, planStatus, codingStatus string,
) {
	t.Helper()
	if details.Job.Status != jobStatus || len(details.Steps) != 3 {
		t.Fatalf("chat pipeline details = %#v", details)
	}
	want := []struct {
		action string
		sort   int
		status string
	}{
		{"objective_resolve", 5, objectiveStatus},
		{"v3_coding_plan", 10, planStatus},
		{"v3_coding", 15, codingStatus},
	}
	for index, expected := range want {
		step := details.Steps[index]
		if step.Action != expected.action || step.SortIndex != expected.sort ||
			step.Status != expected.status || step.Generation != details.Job.CurrentGeneration {
			t.Fatalf(
				"chat pipeline step %d = %s@%d/%s generation %d, want %s@%d/%s generation %d",
				index, step.Action, step.SortIndex, step.Status, step.Generation,
				expected.action, expected.sort, expected.status, details.Job.CurrentGeneration,
			)
		}
	}
}

package queue

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresConversationCandidatesBindPriorUserAndAssistantResult(t *testing.T) {
	ctx, repository := channelTurnTestRepository(t)
	channel, err := repository.CreateChannel(ctx, model.Channel{
		ID: "candidate-authority", Scope: model.ChannelScopeUser, Mode: model.ChannelModeAssistant, Name: "Candidate authority",
		WorkspaceRoot: "/srv/workspaces/candidate-authority",
	})
	if err != nil {
		t.Fatal(err)
	}
	firstUser, firstJob, err := repository.EnqueueChannelTurn(ctx, channel.ID, "Compare the two implementations.")
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "candidate-first-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != firstJob.ID {
		t.Fatalf("claim=%+v want job %d", claim, firstJob.ID)
	}
	firstResult := "The second implementation has lower latency."
	if err := repository.CompleteStep(ctx, CompleteStepCommand{
		OperationID: testLifecycleOperationID(t, "candidate-first-complete", claim.Step.ID),
		Authority:   claim.Authority, StepID: claim.Step.ID, Output: firstResult,
	}); err != nil {
		t.Fatal(err)
	}
	_, secondJob, err := repository.EnqueueChannelTurn(ctx, channel.ID, "Use the second one you recommended.")
	if err != nil {
		t.Fatal(err)
	}
	set, err := repository.ConversationCandidateAuthorities(ctx, secondJob)
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Turns) != 2 || set.Turns[0].MessageID != firstUser.ID ||
		set.Turns[0].Role != assemblyline.ConversationContextUser ||
		set.Turns[1].Role != assemblyline.ConversationContextAssistant {
		t.Fatalf("candidate turns=%+v", set.Turns)
	}
	if len(set.AssistantResults) != 1 {
		t.Fatalf("assistant results=%+v", set.AssistantResults)
	}
	paired := set.AssistantResults[0]
	if paired.UserMessageID != firstUser.ID || paired.MessageID != set.Turns[1].MessageID ||
		paired.JobID != firstJob.ID || paired.Content != firstResult {
		t.Fatalf("paired result=%+v first_user=%+v first_job=%+v", paired, firstUser, firstJob)
	}
}

func TestPostgresConversationCandidatesRejectForeignFutureAndMissingCurrentAuthority(t *testing.T) {
	ctx, repository := channelTurnTestRepository(t)
	first, err := repository.CreateChannel(ctx, model.Channel{
		ID: "candidate-first", Scope: model.ChannelScopeUser, Mode: model.ChannelModeAssistant, Name: "Candidate first",
		WorkspaceRoot: "/srv/workspaces/candidate-first",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := repository.CreateChannel(ctx, model.Channel{
		ID: "candidate-second", Scope: model.ChannelScopeUser, Mode: model.ChannelModeAssistant, Name: "Candidate second",
		WorkspaceRoot: "/srv/workspaces/candidate-second",
	})
	if err != nil {
		t.Fatal(err)
	}
	message, job, err := repository.EnqueueChannelTurn(ctx, first.ID, "Current exact authority.")
	if err != nil {
		t.Fatal(err)
	}
	var binding channelTurnMetadata
	if err := json.Unmarshal(job.Metadata, &binding); err != nil {
		t.Fatal(err)
	}
	foreign := binding
	foreign.ChannelID = second.ID
	foreign.SessionID = "channel:" + string(second.ID)
	for name, malformed := range map[string]channelTurnMetadata{
		"foreign_channel": foreign,
		"future_message": func() channelTurnMetadata {
			value := binding
			value.ChannelUserMessageID = message.ID + 100
			return value
		}(),
		"missing_message": func() channelTurnMetadata {
			value := binding
			value.ChannelUserMessageID = 1
			return value
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(malformed)
			if err != nil {
				t.Fatal(err)
			}
			copy := job
			copy.Metadata = raw
			if _, err := repository.ConversationCandidateAuthorities(ctx, copy); err == nil {
				t.Fatalf("%s current authority was accepted", name)
			}
		})
	}
}

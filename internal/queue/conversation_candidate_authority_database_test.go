package queue

import (
	"encoding/json"
	"strings"
	"testing"

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
		set.Turns[0].Role != ConversationCandidateUser ||
		set.Turns[1].Role != ConversationCandidateAssistant {
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

func TestPostgresConversationCandidatesRetainSixCompleteExchangesBeyondFormerRawByteBound(t *testing.T) {
	ctx, repository := channelTurnTestRepository(t)
	channel, err := repository.CreateChannel(ctx, model.Channel{
		ID: "candidate-complete-suffix", Scope: model.ChannelScopeUser,
		Mode: model.ChannelModeAssistant, Name: "Candidate complete suffix",
		WorkspaceRoot: "/srv/workspaces/candidate-complete-suffix",
	})
	if err != nil {
		t.Fatal(err)
	}
	const formerRawByteBound = 6 * 1024
	longResponse := strings.Repeat("r", formerRawByteBound+1)
	wideExchangeUser := strings.Repeat("u", 1024)
	wideExchangeResponse := strings.Repeat("e", formerRawByteBound-len(wideExchangeUser)+1)
	if len(longResponse) <= formerRawByteBound || len(wideExchangeResponse) >= formerRawByteBound ||
		len(wideExchangeUser)+len(wideExchangeResponse) <= formerRawByteBound {
		t.Fatal("long conversation candidate fixture does not cross the former raw byte bound")
	}
	type completedExchange struct {
		user   model.ChannelMessage
		job    model.Job
		output string
	}
	fixtures := []struct {
		instruction string
		output      string
	}{
		{instruction: "This oldest completed exchange must leave the six-exchange suffix.", output: "Oldest result."},
		{instruction: "Retain the valid long response.", output: longResponse},
		{instruction: wideExchangeUser, output: wideExchangeResponse},
		{instruction: "Retain completed exchange four.", output: "Completed result four."},
		{instruction: "Retain completed exchange five.", output: "Completed result five."},
		{instruction: "Retain completed exchange six.", output: "Completed result six."},
		{instruction: "Retain completed exchange seven.", output: "Completed result seven."},
	}
	completed := make([]completedExchange, 0, len(fixtures))
	for _, fixture := range fixtures {
		user, job := completeConversationCandidateExchange(
			t, repository, channel.ID, fixture.instruction, fixture.output,
		)
		completed = append(completed, completedExchange{user: user, job: job, output: fixture.output})
	}
	failedUser, failedJob, err := repository.EnqueueChannelTurn(
		ctx, channel.ID, "This failed turn must not become an orphan user candidate.",
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "candidate-failed-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != failedJob.ID {
		t.Fatalf("claim=%+v want failed job %d", claim, failedJob.ID)
	}
	if err := repository.FailStep(ctx, FailStepCommand{
		OperationID: testLifecycleOperationID(t, "candidate-failed", claim.Step.ID),
		Authority:   claim.Authority, StepID: claim.Step.ID, Error: "expected candidate fixture failure",
	}); err != nil {
		t.Fatal(err)
	}
	_, currentJob, err := repository.EnqueueChannelTurn(
		ctx, channel.ID, "Use the retained completed exchanges.",
	)
	if err != nil {
		t.Fatal(err)
	}
	set, err := repository.ConversationCandidateAuthorities(ctx, currentJob)
	if err != nil {
		t.Fatal(err)
	}
	want := completed[1:]
	if len(set.Turns) != len(want)*2 || len(set.AssistantResults) != len(want) {
		t.Fatalf("candidate turns/results=%d/%d want %d/%d", len(set.Turns), len(set.AssistantResults), len(want)*2, len(want))
	}
	for index, exchange := range want {
		user := set.Turns[index*2]
		assistant := set.Turns[index*2+1]
		result := set.AssistantResults[index]
		if user.MessageID != exchange.user.ID || user.Role != ConversationCandidateUser ||
			user.Content != exchange.user.Content || user.PairedUserMessageID != 0 {
			t.Fatalf("candidate user %d=%+v want message %d", index, user, exchange.user.ID)
		}
		if assistant.Role != ConversationCandidateAssistant ||
			assistant.PairedUserMessageID != user.MessageID || assistant.Content != exchange.output {
			t.Fatalf("candidate assistant %d=%+v user=%+v", index, assistant, user)
		}
		if result.UserMessageID != user.MessageID || result.MessageID != assistant.MessageID ||
			result.JobID != exchange.job.ID || result.Content != exchange.output {
			t.Fatalf("candidate result %d=%+v exchange=%+v", index, result, exchange)
		}
		if user.MessageID == failedUser.ID || assistant.MessageID == failedUser.ID {
			t.Fatalf("failed user message %d entered complete candidate suffix", failedUser.ID)
		}
	}
	if set.Turns[0].MessageID == completed[0].user.ID {
		t.Fatalf("seventh-oldest exchange %d entered six-exchange suffix", completed[0].user.ID)
	}
}

func TestPostgresConversationCandidatesRejectNonAdjacentAssistantFragment(t *testing.T) {
	ctx, repository := channelTurnTestRepository(t)
	channel, err := repository.CreateChannel(ctx, model.Channel{
		ID: "candidate-orphan-assistant", Scope: model.ChannelScopeUser,
		Mode: model.ChannelModeAssistant, Name: "Candidate orphan assistant",
		WorkspaceRoot: "/srv/workspaces/candidate-orphan-assistant",
	})
	if err != nil {
		t.Fatal(err)
	}
	completeConversationCandidateExchange(
		t, repository, channel.ID, "Create one valid exchange.", "Valid assistant result.",
	)
	if _, err := repository.pool.Exec(ctx, `
		INSERT INTO ai_channel_messages(channel_id,role,content)
		VALUES ($1,'assistant','Unbound assistant fragment.')
	`, channel.ID); err != nil {
		t.Fatal(err)
	}
	_, currentJob, err := repository.EnqueueChannelTurn(ctx, channel.ID, "Load exact complete history.")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ConversationCandidateAuthorities(ctx, currentJob); err == nil ||
		!strings.Contains(err.Error(), "not immediately preceded by a user authority") {
		t.Fatalf("non-adjacent assistant fragment error=%v", err)
	}
}

func completeConversationCandidateExchange(
	t *testing.T,
	repository *Repository,
	channelID model.ChannelID,
	instruction string,
	output string,
) (model.ChannelMessage, model.Job) {
	t.Helper()
	user, job, err := repository.EnqueueChannelTurn(t.Context(), channelID, instruction)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(t.Context(), "candidate-complete-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%+v want job %d", claim, job.ID)
	}
	if err := repository.CompleteStep(t.Context(), CompleteStepCommand{
		OperationID: testLifecycleOperationID(t, "candidate-complete", claim.Step.ID),
		Authority:   claim.Authority, StepID: claim.Step.ID, Output: output,
	}); err != nil {
		t.Fatal(err)
	}
	return user, job
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
		"absent_message": func() channelTurnMetadata {
			value := binding
			value.ChannelUserMessageID = message.ID + 1
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

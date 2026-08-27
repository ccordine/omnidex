package queue

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestSearchConversationContextRecordsFindsOlderSameChannelAuthority(t *testing.T) {
	ctx, repository := channelTurnTestRepository(t)
	channel, err := repository.CreateChannel(ctx, model.Channel{
		ID: "context-search-primary", Scope: model.ChannelScopeUser,
		Mode: model.ChannelModeAssistant, Name: "Context search primary",
		WorkspaceRoot: "/srv/workspaces/context-search-primary",
	})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := repository.CreateChannel(ctx, model.Channel{
		ID: "context-search-foreign", Scope: model.ChannelScopeUser,
		Mode: model.ChannelModeAssistant, Name: "Context search foreign",
		WorkspaceRoot: "/srv/workspaces/context-search-foreign",
	})
	if err != nil {
		t.Fatal(err)
	}
	completeContextSearchTurn(
		t, repository, foreign.ID,
		"Remember the foreign cobalt antenna setting.",
		"The foreign cobalt antenna uses setting ninety-nine.",
	)
	relevantMessage, _ := completeContextSearchTurn(
		t, repository, channel.ID,
		"Rotate the cobalt antenna toward Earth.",
		"Alignment is complete with the aphelion confirmation.",
	)
	for index := 0; index < 9; index++ {
		completeContextSearchTurn(
			t, repository, channel.ID,
			fmt.Sprintf("Record filler sample %d.", index),
			fmt.Sprintf("Filler sample %d is recorded.", index),
		)
	}
	_, currentJob, err := repository.EnqueueChannelTurn(ctx, channel.ID, "Do it again.")
	if err != nil {
		t.Fatal(err)
	}
	wantSourcePrefix := fmt.Sprintf("channel-message-%d-through-", relevantMessage.ID)
	for _, terms := range [][]string{{"cobalt antenna"}, {"aphelion confirmation"}} {
		records, err := repository.SearchConversationContextRecords(ctx, currentJob, terms, 8)
		if err != nil {
			t.Fatal(err)
		}
		if len(records) != 1 {
			t.Fatalf("search %q records=%#v, want the old same-channel exchange only", terms, records)
		}
		record := records[0]
		if strings.Contains(record.Content, "foreign") || strings.Contains(record.Content, "ninety-nine") {
			t.Fatalf("foreign channel authority leaked into search: %#v", records)
		}
		if !strings.HasPrefix(record.SourceID, wantSourcePrefix) ||
			record.Namespace != "conversation_exchange" ||
			record.Content != "user message:\nRotate the cobalt antenna toward Earth.\n"+
				"assistant response:\nAlignment is complete with the aphelion confirmation." {
			t.Fatalf("searched exchange lost exact question/answer authority: %#v", record)
		}
	}
}

func completeContextSearchTurn(
	t *testing.T,
	repository *Repository,
	channelID model.ChannelID,
	instruction, output string,
) (model.ChannelMessage, model.Job) {
	t.Helper()
	message, job, err := repository.EnqueueChannelTurn(t.Context(), channelID, instruction)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(t.Context(), "context-search-proof")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claimed job=%+v, want %d", claim, job.ID)
	}
	if err := repository.CompleteStep(t.Context(), CompleteStepCommand{
		OperationID: testLifecycleOperationID(t, "context-search-complete", claim.Step.ID),
		Authority:   claim.Authority, StepID: claim.Step.ID, Output: output,
	}); err != nil {
		t.Fatal(err)
	}
	return message, job
}

package queue

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/sessionintent"
)

func TestResearchSeparateDurableConversationAndReplay(t *testing.T) {
	_, repo := freshUserIntentRepository(t)
	command, _ := userIntentFixture(t, repo, "Retain this planning job unchanged.")
	ctx := t.Context()
	before, err := repo.ChannelSessionState(ctx, command.ChannelID, command.WorkspaceIdentity)
	if err != nil {
		t.Fatal(err)
	}
	request := sessionintent.ResearchRequest{OperationID: "lifecycle_operation_research_one", WorkspaceIdentity: command.WorkspaceIdentity, Text: "Read the local contract and compare web sources."}
	calls := 0
	answer := func(ctx context.Context, current sessionintent.ResearchContext) (string, json.RawMessage, error) {
		calls++
		if !current.State.Equal(before) || len(current.Recent) != calls-1 {
			t.Fatalf("wrong research context: %+v", current)
		}
		// Main workflow reads remain available during the separate research call.
		if _, err := repo.ChannelSessionState(ctx, command.ChannelID, command.WorkspaceIdentity); err != nil {
			t.Fatal(err)
		}
		return "Observed source-backed answer.", json.RawMessage(`{"provider":"fixture"}`), nil
	}
	first, err := repo.Research(ctx, command.ChannelID, request, answer)
	if err != nil {
		t.Fatal(err)
	}
	again, err := repo.Research(ctx, command.ChannelID, request, answer)
	if err != nil || calls != 1 || !reflect.DeepEqual(first, again) {
		t.Fatal(again, err, calls)
	}
	request.Text = "Changed bytes."
	if _, err := repo.Research(ctx, command.ChannelID, request, answer); !errors.Is(err, ErrLifecycleOperationConflict) {
		t.Fatal("changed replay accepted", err)
	}
	request.OperationID = "lifecycle_operation_research_two"
	if _, err := repo.Research(ctx, command.ChannelID, request, answer); err != nil {
		t.Fatal(err)
	}
	history, err := repo.ResearchHistory(ctx, command.ChannelID, command.WorkspaceIdentity)
	if err != nil || len(history) != 2 || history[0] != first {
		t.Fatal(history, err)
	}
	after, err := repo.ChannelSessionState(ctx, command.ChannelID, command.WorkspaceIdentity)
	if err != nil || !after.Equal(before) {
		t.Fatal("research changed main workflow", after, err)
	}
	current, err := userIntentContextTxForResearchTest(ctx, repo, command.ChannelID, command.WorkspaceIdentity)
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range current.Recent {
		if message.Text == first.Question || message.Text == first.Answer {
			t.Fatal("research leaked into interpreter history")
		}
	}
	request.OperationID = "lifecycle_operation_research_failed"
	failed, err := repo.Research(ctx, command.ChannelID, request, func(context.Context, sessionintent.ResearchContext) (string, json.RawMessage, error) {
		return "", json.RawMessage(`[]`), errors.New("provider unavailable")
	})
	if err != nil || failed.Error == "" || failed.Answer != "" {
		t.Fatal(failed, err)
	}
	request.WorkspaceIdentity = "wrong-workspace"
	if _, err := repo.Research(ctx, command.ChannelID, request, answer); err == nil {
		t.Fatal("wrong workspace accepted")
	}
}

func userIntentContextTxForResearchTest(ctx context.Context, repo *Repository, channel model.ChannelID, identity string) (sessionintent.Context, error) {
	tx, err := repo.pool.Begin(ctx)
	if err != nil {
		return sessionintent.Context{}, err
	}
	defer tx.Rollback(ctx)
	return userIntentContextTx(ctx, tx, channel, identity)
}

func TestResearchForkReconnectAndCancellationRetainOrigin(t *testing.T) {
	_, repo := freshUserIntentRepository(t)
	command, fixture := userIntentFixture(t, repo, "Keep coding approval unchanged")
	before, err := repo.ChannelSessionState(t.Context(), command.ChannelID, command.WorkspaceIdentity)
	if err != nil {
		t.Fatal(err)
	}
	id := model.LifecycleOperationID("lifecycle_operation_fork_messages")
	request := sessionintent.ResearchRequest{OperationID: id, ConversationID: id, WorkspaceIdentity: command.WorkspaceIdentity, Text: "Inspect selected context", Fork: &sessionintent.ResearchSeed{SourceChannel: command.ChannelID, MessageIDs: []int64{*before.LatestMessageID}}}
	answer := func(ctx context.Context, current sessionintent.ResearchContext) (string, json.RawMessage, error) {
		if current.Fork == nil || current.Fork.SourceChannel != command.ChannelID || len(current.Fork.Messages) != 1 || current.Fork.Messages[0].ID != *before.LatestMessageID {
			t.Fatalf("source lost: %+v", current)
		}
		return "Selected source finding", json.RawMessage(`[]`), nil
	}
	first, err := repo.Research(t.Context(), command.ChannelID, request, answer)
	if err != nil {
		t.Fatal(err)
	}
	// New repository object represents reconnection to the persisted conversation.
	reconnect := New(repo.pool, repo.modelAuthority)
	repo = reconnect
	history, err := repo.ResearchConversationHistory(t.Context(), command.ChannelID, command.WorkspaceIdentity, id)
	if err != nil || len(history) != 1 || !reflect.DeepEqual(history[0], first) {
		t.Fatal(history, err)
	}
	request.OperationID = "lifecycle_operation_fork_followup"
	request.Fork = nil
	if _, err := repo.Research(t.Context(), command.ChannelID, request, answer); err != nil {
		t.Fatal(err)
	}
	main, err := repo.ResearchHistory(t.Context(), command.ChannelID, command.WorkspaceIdentity)
	if err != nil || len(main) != 0 {
		t.Fatal("fork leaked into main Research", main, err)
	}
	request.OperationID = "lifecycle_operation_fork_canceled"
	ctx, cancel := context.WithCancel(t.Context())
	canceled, err := repo.Research(ctx, command.ChannelID, request, func(ctx context.Context, current sessionintent.ResearchContext) (string, json.RawMessage, error) {
		cancel()
		return "", json.RawMessage(`[]`), ctx.Err()
	})
	if err != nil || canceled.Error != "context canceled" {
		t.Fatal(canceled, err)
	}
	replay, err := repo.Research(t.Context(), command.ChannelID, request, answer)
	if err != nil || !reflect.DeepEqual(replay, canceled) {
		t.Fatal(replay, err)
	}
	request.OperationID = "lifecycle_operation_fork_ledger"
	request.ConversationID = request.OperationID
	request.Fork = &sessionintent.ResearchSeed{SourceChannel: command.ChannelID, LedgerID: fixture.created.LedgerID, LedgerRevision: fixture.created.Revision}
	ledger, err := repo.Research(t.Context(), command.ChannelID, request, func(ctx context.Context, current sessionintent.ResearchContext) (string, json.RawMessage, error) {
		if current.Fork.LedgerRevision != fixture.created.Revision || !json.Valid(current.Fork.Ledger) {
			t.Fatalf("ledger source lost: %+v", current.Fork)
		}
		return "Ledger finding", json.RawMessage(`[]`), nil
	})
	if err != nil || ledger.Fork.LedgerID != fixture.created.LedgerID {
		t.Fatal(ledger, err)
	}
	after, err := repo.ChannelSessionState(t.Context(), command.ChannelID, command.WorkspaceIdentity)
	if err != nil || !after.Equal(before) {
		t.Fatal("Research changed coding state", after, err)
	}
	request.OperationID = "lifecycle_operation_fork_spoof"
	request.ConversationID = request.OperationID
	request.Fork = &sessionintent.ResearchSeed{SourceChannel: "other", MessageIDs: []int64{*before.LatestMessageID}}
	if _, err := repo.Research(t.Context(), command.ChannelID, request, answer); err == nil {
		t.Fatal("foreign source accepted")
	}
}

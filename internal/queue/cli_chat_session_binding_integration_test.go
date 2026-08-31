package queue_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestCLIChatSessionRejectsReplacedIdentityBeforeHistoryOrEnqueue(t *testing.T) {
	databaseURL := os.Getenv("OMNI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for isolated PostgreSQL CLI session coverage")
	}

	_, repository := freshLifecycleRepository(t, databaseURL)
	ctx := context.Background()
	workspaceRoot := "/tmp/omnidex-cli-binding-" + lifecycleNonce(t)
	identityA := "directory_identity_v1_" + strings.Repeat("a", 64)
	identityB := "directory_identity_v1_" + strings.Repeat("b", 64)
	channel, err := repository.EnsureCLIChatSessionChannel(ctx, workspaceRoot, identityA)
	if err != nil {
		t.Fatalf("create exact CLI session: %v", err)
	}

	if _, err := repository.ChannelSessionState(
		ctx,
		channel.ID,
		identityB,
	); !errors.Is(err, queue.ErrChannelSessionWorkspace) {
		t.Fatalf("replaced-identity state error = %v, want ErrChannelSessionWorkspace", err)
	}
	if _, err := repository.ChannelSessionSnapshot(
		ctx,
		channel.ID,
		queue.MaxChannelSessionMessages,
		identityB,
	); !errors.Is(err, queue.ErrChannelSessionWorkspace) {
		t.Fatalf("replaced-identity snapshot error = %v, want ErrChannelSessionWorkspace", err)
	}
	operationID, err := queue.NewLifecycleOperationID(
		"cli-binding-integration",
		lifecycleNonce(t),
	)
	if err != nil {
		t.Fatalf("create session operation ID: %v", err)
	}
	if _, err := repository.SubmitChannelSessionTurn(
		ctx,
		queue.ChannelSessionTurnCommand{
			OperationID:       operationID,
			ChannelID:         channel.ID,
			WorkspaceRoot:     workspaceRoot,
			WorkspaceIdentity: identityB,
			Text:              "This turn must not enter the replaced workspace session.",
		},
	); !errors.Is(err, queue.ErrChannelSessionWorkspace) {
		t.Fatalf("replaced-identity turn error = %v, want ErrChannelSessionWorkspace", err)
	}

	state, err := repository.ChannelSessionState(ctx, channel.ID, identityA)
	if err != nil {
		t.Fatalf("read original empty CLI state: %v", err)
	}
	if state.LatestMessageID != nil || state.LatestTurnOperationID != nil ||
		state.LatestControlOperationID != nil || state.LatestJob != nil {
		t.Fatalf("rejected turn changed original CLI state: %#v", state)
	}
	snapshot, err := repository.ChannelSessionSnapshot(
		ctx,
		channel.ID,
		queue.MaxChannelSessionMessages,
		identityA,
	)
	if err != nil {
		t.Fatalf("read original empty CLI snapshot: %v", err)
	}
	if len(snapshot.Transcript.Messages) != 0 || len(snapshot.Turns) != 0 ||
		len(snapshot.Controls) != 0 || snapshot.ActiveJob != nil {
		t.Fatalf("rejected turn created CLI history or job: %#v", snapshot)
	}
}

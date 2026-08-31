package main

import (
	"context"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

type chatStatePollResult struct {
	snapshotRevision uint64
	state            client.ChatSessionState
	err              error
}

func startChatStatePoll(
	ctx context.Context,
	apiClient *client.Client,
	channel model.Channel,
	workspaceIdentity string,
	snapshotRevision uint64,
	results chan<- chatStatePollResult,
) {
	go func() {
		state, err := apiClient.ChatSessionState(ctx, channel, workspaceIdentity)
		result := chatStatePollResult{
			snapshotRevision: snapshotRevision,
			state:            state,
			err:              err,
		}
		select {
		case <-ctx.Done():
		case results <- result:
		}
	}()
}

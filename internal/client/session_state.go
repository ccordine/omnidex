package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
)

const maxChatSessionStateResponseBytes int64 = 64 * 1024

func (client *Client) ChatSessionState(
	ctx context.Context,
	expected model.Channel,
	workspaceIdentity string,
) (model.ChannelSessionState, error) {
	if _, err := requireExactCLIChannel(expected, expected); err != nil {
		return model.ChannelSessionState{}, err
	}
	if err := projectroot.ValidateClientWorkspaceIdentity(workspaceIdentity); err != nil {
		return model.ChannelSessionState{}, fmt.Errorf("chat session state workspace identity: %w", err)
	}
	var state model.ChannelSessionState
	query := url.Values{}
	query.Set("workspace_identity", workspaceIdentity)
	requestPath := "/v1/channels/" + string(expected.ID) + "/session/state?" + query.Encode()
	if err := client.doJSONBounded(
		ctx,
		http.MethodGet,
		requestPath,
		nil,
		&state,
		http.StatusOK,
		maxChatSessionStateResponseBytes,
	); err != nil {
		return model.ChannelSessionState{}, err
	}
	if err := validateChatSessionState(expected, workspaceIdentity, state); err != nil {
		return model.ChannelSessionState{}, err
	}
	return state, nil
}

func validateChatSessionState(
	expected model.Channel,
	workspaceIdentity string,
	state model.ChannelSessionState,
) error {
	if state.ChannelID != expected.ID || state.WorkspaceRoot != expected.WorkspaceRoot ||
		state.WorkspaceIdentity != workspaceIdentity {
		return fmt.Errorf("chat session state differs from exact CLI channel authority")
	}
	if state.ChannelUpdatedAt.IsZero() {
		return fmt.Errorf("chat session state has no channel update timestamp")
	}
	if state.LatestMessageID != nil && *state.LatestMessageID < 1 {
		return fmt.Errorf("chat session state has an invalid latest message identity")
	}
	for label, operationID := range map[string]*model.LifecycleOperationID{
		"turn": state.LatestTurnOperationID, "control": state.LatestControlOperationID,
	} {
		if operationID == nil {
			continue
		}
		if _, err := model.ParseLifecycleOperationID(string(*operationID)); err != nil {
			return fmt.Errorf("chat session latest %s operation: %w", label, err)
		}
	}
	if state.LatestJob == nil {
		return nil
	}
	job := state.LatestJob
	if job.ID < 1 || job.Generation < 1 || job.UpdatedAt.IsZero() {
		return fmt.Errorf("chat session state has incomplete latest job authority")
	}
	switch job.Status {
	case model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting,
		model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCanceled:
		return nil
	default:
		return fmt.Errorf("chat session state has unsupported job status %q", job.Status)
	}
}

package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/queue"
)

const maxChatSessionStateResponseBytes int64 = 64 * 1024

type ChatSessionJobState struct {
	ID         int64     `json:"id"`
	Status     string    `json:"status"`
	Generation int64     `json:"generation"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type ChatSessionState struct {
	ChannelID                model.ChannelID             `json:"channel_id"`
	WorkspaceRoot            string                      `json:"workspace_root"`
	WorkspaceIdentity        string                      `json:"workspace_identity"`
	Revision                 string                      `json:"revision"`
	LatestMessageID          *int64                      `json:"latest_message_id,omitempty"`
	LatestTurnOperationID    *queue.LifecycleOperationID `json:"latest_turn_operation_id,omitempty"`
	LatestControlOperationID *queue.LifecycleOperationID `json:"latest_control_operation_id,omitempty"`
	LatestJob                *ChatSessionJobState        `json:"latest_job,omitempty"`
}

func (client *Client) ChatSessionState(
	ctx context.Context,
	expected model.Channel,
	workspaceIdentity string,
) (ChatSessionState, error) {
	if _, err := requireExactCLIChannel(expected, expected); err != nil {
		return ChatSessionState{}, err
	}
	if err := projectroot.ValidateDirectoryIdentity(workspaceIdentity); err != nil {
		return ChatSessionState{}, fmt.Errorf("chat session state workspace identity: %w", err)
	}
	var state ChatSessionState
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
		return ChatSessionState{}, err
	}
	if err := validateChatSessionState(expected, workspaceIdentity, state); err != nil {
		return ChatSessionState{}, err
	}
	return state, nil
}

func validateChatSessionState(
	expected model.Channel,
	workspaceIdentity string,
	state ChatSessionState,
) error {
	if state.ChannelID != expected.ID || state.WorkspaceRoot != expected.WorkspaceRoot ||
		state.WorkspaceIdentity != workspaceIdentity {
		return fmt.Errorf("chat session state differs from exact CLI channel authority")
	}
	if !canonicalSessionRevision(state.Revision) {
		return fmt.Errorf("chat session state has an invalid revision")
	}
	if state.LatestMessageID != nil && *state.LatestMessageID < 1 {
		return fmt.Errorf("chat session state has an invalid latest message identity")
	}
	for label, operationID := range map[string]*queue.LifecycleOperationID{
		"turn": state.LatestTurnOperationID, "control": state.LatestControlOperationID,
	} {
		if operationID == nil {
			continue
		}
		if _, err := queue.ParseLifecycleOperationID(string(*operationID)); err != nil {
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

func canonicalSessionRevision(value string) bool {
	const prefix = "channel_session_revision_"
	digest := strings.TrimPrefix(value, prefix)
	if len(digest) != 64 || len(value) != len(prefix)+64 {
		return false
	}
	for _, character := range []byte(digest) {
		if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}

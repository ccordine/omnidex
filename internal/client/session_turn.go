package client

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/queue"
)

type SessionTurnReceipt struct {
	OperationID       queue.LifecycleOperationID          `json:"operation_id"`
	Disposition       queue.ChannelSessionTurnDisposition `json:"disposition"`
	ChannelID         model.ChannelID                     `json:"channel_id"`
	WorkspaceRoot     string                              `json:"workspace_root"`
	WorkspaceIdentity string                              `json:"workspace_identity"`
	JobID             int64                               `json:"job_id"`
	Status            string                              `json:"status"`
	Generation        int64                               `json:"generation"`
	UserMessage       *model.ChannelMessage               `json:"user_message,omitempty"`
}

func (client *Client) SubmitSessionTurn(
	ctx context.Context,
	channel model.Channel,
	workspaceIdentity string,
	operationID queue.LifecycleOperationID,
	exactText string,
) (SessionTurnReceipt, error) {
	if err := channel.ValidateStored(); err != nil {
		return SessionTurnReceipt{}, fmt.Errorf("invalid CLI chat channel: %w", err)
	}
	if channel.Scope != model.ChannelScopeUser || channel.Mode != model.ChannelModeAssistant ||
		channel.DataSourceID != "" || channel.RoleplayViewpointCharacterID != "" {
		return SessionTurnReceipt{}, fmt.Errorf("CLI chat channel has unsupported authority")
	}
	if _, err := queue.ParseLifecycleOperationID(string(operationID)); err != nil {
		return SessionTurnReceipt{}, err
	}
	if err := projectroot.ValidateDirectoryIdentity(workspaceIdentity); err != nil {
		return SessionTurnReceipt{}, fmt.Errorf("session turn workspace identity: %w", err)
	}
	if err := ValidateSessionTurnText(exactText); err != nil {
		return SessionTurnReceipt{}, err
	}
	payload := struct {
		WorkspaceRoot     string                     `json:"workspace_root"`
		WorkspaceIdentity string                     `json:"workspace_identity"`
		Text              string                     `json:"text"`
		OperationID       queue.LifecycleOperationID `json:"operation_id"`
	}{
		WorkspaceRoot:     channel.WorkspaceRoot,
		WorkspaceIdentity: workspaceIdentity,
		Text:              exactText,
		OperationID:       operationID,
	}
	var receipt SessionTurnReceipt
	requestPath := "/v1/channels/" + string(channel.ID) + "/session/turn"
	if err := client.doJSON(
		ctx,
		http.MethodPost,
		requestPath,
		payload,
		&receipt,
		http.StatusAccepted,
	); err != nil {
		return SessionTurnReceipt{}, err
	}
	if err := validateSessionTurnReceipt(
		channel,
		workspaceIdentity,
		operationID,
		exactText,
		receipt,
	); err != nil {
		return SessionTurnReceipt{}, err
	}
	return receipt, nil
}

func validateSessionTurnReceipt(
	channel model.Channel,
	workspaceIdentity string,
	operationID queue.LifecycleOperationID,
	exactText string,
	receipt SessionTurnReceipt,
) error {
	if receipt.OperationID != operationID || receipt.ChannelID != channel.ID ||
		receipt.WorkspaceRoot != channel.WorkspaceRoot ||
		receipt.WorkspaceIdentity != workspaceIdentity ||
		receipt.JobID < 1 || receipt.Generation < 1 {
		return fmt.Errorf("session turn receipt differs from exact channel operation authority")
	}
	switch receipt.Disposition {
	case queue.ChannelSessionTurnEnqueued:
		if receipt.Status != model.JobStatusPending || receipt.UserMessage == nil ||
			receipt.UserMessage.ID < 1 || receipt.UserMessage.ChannelID != channel.ID ||
			receipt.UserMessage.Role != model.ChannelMessageRoleUser ||
			receipt.UserMessage.Content != exactText {
			return fmt.Errorf("session enqueue receipt differs from exact user turn")
		}
	case queue.ChannelSessionTurnReplanned:
		if receipt.Status != model.JobStatusRunning || receipt.UserMessage != nil {
			return fmt.Errorf("session replan receipt has contradictory result authority")
		}
	case queue.ChannelSessionTurnFeedback:
		if receipt.Status != model.JobStatusRunning && receipt.Status != model.JobStatusCompleted {
			return fmt.Errorf("session feedback receipt has unsupported status %q", receipt.Status)
		}
		if receipt.UserMessage != nil {
			return fmt.Errorf("session feedback receipt unexpectedly carries an enqueue message")
		}
	default:
		return fmt.Errorf("session turn receipt has unsupported disposition %q", receipt.Disposition)
	}
	return nil
}

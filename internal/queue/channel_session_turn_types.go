package queue

import (
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
)

type ChannelSessionTurnDisposition string

const (
	ChannelSessionTurnEnqueued  ChannelSessionTurnDisposition = "enqueued"
	ChannelSessionTurnReplanned ChannelSessionTurnDisposition = "replanned"
	ChannelSessionTurnFeedback  ChannelSessionTurnDisposition = "feedback_submitted"
)

type ChannelSessionTurnCommand struct {
	OperationID       LifecycleOperationID `json:"operation_id"`
	ChannelID         model.ChannelID      `json:"channel_id"`
	WorkspaceRoot     string               `json:"workspace_root"`
	WorkspaceIdentity string               `json:"workspace_identity"`
	Text              string               `json:"text"`
}

type ChannelSessionTurnResult struct {
	OperationID       LifecycleOperationID
	Disposition       ChannelSessionTurnDisposition
	ChannelID         model.ChannelID
	WorkspaceRoot     string
	WorkspaceIdentity string
	Job               model.Job
	UserMessage       *model.ChannelMessage
	Applied           bool
}

func normalizeChannelSessionTurnCommand(
	command ChannelSessionTurnCommand,
) (ChannelSessionTurnCommand, error) {
	operationID, err := ParseLifecycleOperationID(string(command.OperationID))
	if err != nil {
		return ChannelSessionTurnCommand{}, err
	}
	if err := command.ChannelID.Validate(); err != nil {
		return ChannelSessionTurnCommand{}, err
	}
	if err := model.ValidateChannelWorkspaceRoot(command.WorkspaceRoot); err != nil {
		return ChannelSessionTurnCommand{}, err
	}
	if err := projectroot.ValidateDirectoryIdentity(command.WorkspaceIdentity); err != nil {
		return ChannelSessionTurnCommand{}, fmt.Errorf("channel session workspace identity: %w", err)
	}
	if err := model.ValidateChannelMessage(model.ChannelMessageRoleUser, command.Text); err != nil {
		return ChannelSessionTurnCommand{}, err
	}
	command.OperationID = operationID
	return command, nil
}

func validateChannelSessionTurnDisposition(disposition ChannelSessionTurnDisposition) error {
	switch disposition {
	case ChannelSessionTurnEnqueued, ChannelSessionTurnReplanned, ChannelSessionTurnFeedback:
		return nil
	default:
		return fmt.Errorf("unregistered channel session turn disposition %q", disposition)
	}
}

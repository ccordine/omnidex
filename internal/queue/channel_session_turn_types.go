package queue

import (
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
)

type ChannelSessionTurnCommand struct {
	OperationID       model.LifecycleOperationID `json:"operation_id"`
	ChannelID         model.ChannelID            `json:"channel_id"`
	WorkspaceRoot     string                     `json:"workspace_root"`
	WorkspaceIdentity string                     `json:"workspace_identity"`
	Text              string                     `json:"text"`
}

type ChannelSessionTurnResult struct {
	OperationID       model.LifecycleOperationID
	Disposition       model.ChannelSessionTurnDisposition
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
	operationID, err := model.ParseLifecycleOperationID(string(command.OperationID))
	if err != nil {
		return ChannelSessionTurnCommand{}, err
	}
	if err := command.ChannelID.Validate(); err != nil {
		return ChannelSessionTurnCommand{}, err
	}
	if err := model.ValidateChannelWorkspaceRoot(command.WorkspaceRoot); err != nil {
		return ChannelSessionTurnCommand{}, err
	}
	if err := projectroot.ValidateClientWorkspaceIdentity(command.WorkspaceIdentity); err != nil {
		return ChannelSessionTurnCommand{}, fmt.Errorf("channel session workspace identity: %w", err)
	}
	if err := model.ValidateChannelMessage(model.ChannelMessageRoleUser, command.Text); err != nil {
		return ChannelSessionTurnCommand{}, err
	}
	command.OperationID = operationID
	return command, nil
}

func validateChannelSessionTurnDisposition(disposition model.ChannelSessionTurnDisposition) error {
	switch disposition {
	case model.ChannelSessionTurnEnqueued, model.ChannelSessionTurnReplanned, model.ChannelSessionTurnFeedback:
		return nil
	default:
		return fmt.Errorf("unregistered channel session turn disposition %q", disposition)
	}
}

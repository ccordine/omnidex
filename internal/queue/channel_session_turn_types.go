package queue

import (
	"fmt"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
)

type ChannelSessionTurnCommand struct {
	OperationID        model.LifecycleOperationID `json:"operation_id"`
	ChannelID          model.ChannelID            `json:"channel_id"`
	WorkspaceRoot      string                     `json:"workspace_root"`
	WorkspaceIdentity  string                     `json:"workspace_identity"`
	Text               string                     `json:"text"`
	CodingMode         codingagent.Mode           `json:"coding_mode,omitempty"`
	ExpectedJobID      *int64                     `json:"expected_job_id,omitempty"`
	ExpectedGeneration *int64                     `json:"expected_generation,omitempty"`
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
	switch command.CodingMode {
	case "":
		command.CodingMode = ""
	case codingagent.Agents:
	default:
		return ChannelSessionTurnCommand{}, fmt.Errorf(
			"%w: unknown coding mode %q", ErrChannelSessionTurnInvalid, command.CodingMode,
		)
	}
	command.OperationID = operationID
	if (command.ExpectedJobID == nil) != (command.ExpectedGeneration == nil) ||
		command.ExpectedJobID != nil && (*command.ExpectedJobID < 1 || *command.ExpectedGeneration < 1) {
		return ChannelSessionTurnCommand{}, fmt.Errorf("%w: active message requires positive observed job and generation", ErrChannelSessionTurnInvalid)
	}
	return command, nil
}

func validateChannelSessionTurnDisposition(disposition model.ChannelSessionTurnDisposition) error {
	switch disposition {
	case model.ChannelSessionTurnEnqueued, model.ChannelSessionTurnReplanned, model.ChannelSessionTurnFeedback, model.ChannelSessionTurnQueued:
		return nil
	default:
		return fmt.Errorf("unregistered channel session turn disposition %q", disposition)
	}
}

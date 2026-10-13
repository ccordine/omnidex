package queue

import (
	"errors"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

var ErrPendingChannelSessionMessage = errors.New("active conversation has an unresolved message")

const channelSessionMessageEvidence = "channel_session_message"

type ChannelSessionMessage struct {
	OperationID         model.LifecycleOperationID `json:"operation_id"`
	Command             ChannelSessionTurnCommand  `json:"command"`
	AdmittedJob         model.Job                  `json:"admitted_job"`
	RedirectOperationID model.LifecycleOperationID `json:"redirect_operation_id"`
}

type ResolveChannelSessionMessageCommand struct {
	Authority   model.StepAttemptAuthority
	OperationID model.LifecycleOperationID
	Kind        assemblyline.ConversationObjectiveKind
	Response    string
}

type FailChannelSessionMessageCommand struct {
	Authority   model.StepAttemptAuthority
	OperationID model.LifecycleOperationID
	Error       string
}

type ChannelSessionMessageResult struct {
	Job             model.Job
	Applied         bool
	Replanned       bool
	ResponseMessage *model.ChannelMessage
}

type channelSessionMessageResolution struct {
	OperationID        model.LifecycleOperationID  `json:"operation_id"`
	Kind               string                      `json:"kind"`
	Job                model.Job                   `json:"job"`
	Authority          *model.StepAttemptAuthority `json:"authority,omitempty"`
	ResponseMessageID  *int64                      `json:"response_message_id,omitempty"`
	ControlOperationID model.LifecycleOperationID  `json:"control_operation_id,omitempty"`
}

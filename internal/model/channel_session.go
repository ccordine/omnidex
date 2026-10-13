package model

import "time"

const MaxChannelSessionTurns = 200
const MaxChannelSessionControls = 200

type ChannelSessionTurnDisposition string

const (
	ChannelSessionTurnEnqueued  ChannelSessionTurnDisposition = "enqueued"
	ChannelSessionTurnReplanned ChannelSessionTurnDisposition = "replanned"
	ChannelSessionTurnFeedback  ChannelSessionTurnDisposition = "feedback_submitted"
	ChannelSessionTurnQueued    ChannelSessionTurnDisposition = "message_queued"
)

type ChannelSessionMessageState string

const (
	ChannelSessionMessagePending    ChannelSessionMessageState = "pending"
	ChannelSessionMessageAnswered   ChannelSessionMessageState = "answer"
	ChannelSessionMessageStory      ChannelSessionMessageState = "story"
	ChannelSessionMessageRedirected ChannelSessionMessageState = "workspace_mutation"
	ChannelSessionMessageRetired    ChannelSessionMessageState = "retired"
	ChannelSessionMessageFailed     ChannelSessionMessageState = "failed"
)

type ChannelSessionTurn struct {
	OperationID  LifecycleOperationID          `json:"operation_id"`
	Disposition  ChannelSessionTurnDisposition `json:"disposition"`
	Text         string                        `json:"text"`
	JobID        int64                         `json:"job_id"`
	Generation   int64                         `json:"generation"`
	Status       string                        `json:"status"`
	CreatedAt    time.Time                     `json:"created_at"`
	MessageState ChannelSessionMessageState    `json:"message_state,omitempty"`
}

type ChannelSessionControlKind string

const (
	ChannelSessionControlInterrupt ChannelSessionControlKind = "interrupt"
	ChannelSessionControlReplan    ChannelSessionControlKind = "replan"
	ChannelSessionControlCancel    ChannelSessionControlKind = "cancel"
)

type ChannelSessionControl struct {
	OperationID LifecycleOperationID      `json:"operation_id"`
	Kind        ChannelSessionControlKind `json:"kind"`
	Text        string                    `json:"text"`
	JobID       int64                     `json:"job_id"`
	Generation  int64                     `json:"generation"`
	Status      string                    `json:"status"`
	CreatedAt   time.Time                 `json:"created_at"`
}

type ChannelSessionJobState struct {
	ID         int64     `json:"id"`
	Status     string    `json:"status"`
	Generation int64     `json:"generation"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type ChannelSessionState struct {
	ChannelID                ChannelID               `json:"channel_id"`
	WorkspaceRoot            string                  `json:"workspace_root"`
	WorkspaceIdentity        string                  `json:"workspace_identity"`
	ChannelUpdatedAt         time.Time               `json:"channel_updated_at"`
	LatestMessageID          *int64                  `json:"latest_message_id,omitempty"`
	LatestTurnOperationID    *LifecycleOperationID   `json:"latest_turn_operation_id,omitempty"`
	LatestControlOperationID *LifecycleOperationID   `json:"latest_control_operation_id,omitempty"`
	LatestJob                *ChannelSessionJobState `json:"latest_job,omitempty"`
}

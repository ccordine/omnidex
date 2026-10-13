package client

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
)

// AnswerAgentQuestion binds a reply to the durable question the client displayed.
// The server validates the same generation and waiting attempt atomically.
func (client *Client) AnswerAgentQuestion(
	ctx context.Context,
	channel model.Channel,
	workspaceIdentity string,
	jobID int64,
	operationID model.LifecycleOperationID,
	answer string,
	pause *model.CodingAgentPause,
) (model.Job, error) {
	if err := validateLifecycleControl(channel, workspaceIdentity, jobID, operationID,
		answer, "question answer", maxCancelReasonBytes); err != nil {
		return model.Job{}, err
	}
	if err := validateAgentQuestion(pause); err != nil {
		return model.Job{}, err
	}
	job, err := client.postLifecycleControl(ctx, channel, workspaceIdentity, jobID, operationID, "feedback",
		struct {
			OperationID            model.LifecycleOperationID `json:"operation_id"`
			Feedback               string                     `json:"feedback"`
			WorkspaceRoot          string                     `json:"workspace_root"`
			WorkspaceIdentity      string                     `json:"workspace_identity"`
			ExpectedGeneration     int64                      `json:"expected_generation"`
			ExpectedWaitingAttempt int64                      `json:"expected_waiting_attempt"`
		}{
			OperationID: operationID, Feedback: answer,
			WorkspaceRoot: channel.WorkspaceRoot, WorkspaceIdentity: workspaceIdentity,
			ExpectedGeneration: pause.Generation, ExpectedWaitingAttempt: pause.WaitingAttempt,
		},
	)
	if err != nil {
		return model.Job{}, err
	}
	if job.Status != model.JobStatusRunning {
		return model.Job{}, fmt.Errorf("question answer did not resume the same waiting job")
	}
	return job, nil
}

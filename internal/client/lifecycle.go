package client

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/queue"
)

const (
	maxObjectiveControlBytes = 2 * 1024
	maxCancelReasonBytes     = 64 * 1024
)

type lifecycleControlReceipt struct {
	JobID       int64                      `json:"job_id"`
	OperationID queue.LifecycleOperationID `json:"operation_id"`
	Status      string                     `json:"status"`
}

func (client *Client) SubmitFeedback(
	ctx context.Context,
	channel model.Channel,
	workspaceIdentity string,
	jobID int64,
	operationID queue.LifecycleOperationID,
	feedback string,
) (model.Job, error) {
	return client.feedbackControl(
		ctx, channel, workspaceIdentity, jobID, operationID,
		"feedback", feedback, maxCancelReasonBytes,
	)
}

func (client *Client) Interrupt(
	ctx context.Context,
	channel model.Channel,
	workspaceIdentity string,
	jobID int64,
	operationID queue.LifecycleOperationID,
	feedback string,
) (model.Job, error) {
	return client.feedbackControl(
		ctx, channel, workspaceIdentity, jobID, operationID,
		"interrupt", feedback, maxObjectiveControlBytes,
	)
}

func (client *Client) Replan(
	ctx context.Context,
	channel model.Channel,
	workspaceIdentity string,
	jobID int64,
	operationID queue.LifecycleOperationID,
	feedback string,
) (model.Job, error) {
	return client.feedbackControl(
		ctx, channel, workspaceIdentity, jobID, operationID,
		"replan", feedback, maxObjectiveControlBytes,
	)
}

func (client *Client) Cancel(
	ctx context.Context,
	channel model.Channel,
	workspaceIdentity string,
	jobID int64,
	operationID queue.LifecycleOperationID,
	reason string,
) (model.Job, error) {
	if err := validateLifecycleControl(
		channel, workspaceIdentity, jobID, operationID,
		reason, "cancel reason", maxCancelReasonBytes,
	); err != nil {
		return model.Job{}, err
	}
	return client.postLifecycleControl(
		ctx, channel, workspaceIdentity, jobID, operationID, "cancel",
		struct {
			OperationID       queue.LifecycleOperationID `json:"operation_id"`
			Reason            string                     `json:"reason"`
			WorkspaceRoot     string                     `json:"workspace_root"`
			WorkspaceIdentity string                     `json:"workspace_identity"`
		}{
			OperationID: operationID, Reason: reason,
			WorkspaceRoot: channel.WorkspaceRoot, WorkspaceIdentity: workspaceIdentity,
		},
	)
}

func (client *Client) feedbackControl(
	ctx context.Context,
	channel model.Channel,
	workspaceIdentity string,
	jobID int64,
	operationID queue.LifecycleOperationID,
	action, feedback string,
	maximum int,
) (model.Job, error) {
	if err := validateLifecycleControl(
		channel, workspaceIdentity, jobID, operationID,
		feedback, action+" feedback", maximum,
	); err != nil {
		return model.Job{}, err
	}
	return client.postLifecycleControl(
		ctx, channel, workspaceIdentity, jobID, operationID, action,
		struct {
			OperationID       queue.LifecycleOperationID `json:"operation_id"`
			Feedback          string                     `json:"feedback"`
			WorkspaceRoot     string                     `json:"workspace_root"`
			WorkspaceIdentity string                     `json:"workspace_identity"`
		}{
			OperationID: operationID, Feedback: feedback,
			WorkspaceRoot: channel.WorkspaceRoot, WorkspaceIdentity: workspaceIdentity,
		},
	)
}

func (client *Client) postLifecycleControl(
	ctx context.Context,
	channel model.Channel,
	workspaceIdentity string,
	jobID int64,
	operationID queue.LifecycleOperationID,
	action string,
	payload any,
) (model.Job, error) {
	if err := validateLifecycleWorkspace(channel, workspaceIdentity); err != nil {
		return model.Job{}, err
	}
	var receipt lifecycleControlReceipt
	path := fmt.Sprintf("/v1/jobs/%d/%s", jobID, action)
	if err := client.doJSON(ctx, http.MethodPost, path, payload, &receipt, http.StatusOK); err != nil {
		return model.Job{}, err
	}
	if receipt.JobID != jobID || receipt.OperationID != operationID {
		return model.Job{}, fmt.Errorf("%s receipt does not match the exact submitted job operation", action)
	}
	expectedStatus := ""
	switch action {
	case "feedback":
		if receipt.Status != model.JobStatusRunning && receipt.Status != model.JobStatusCompleted {
			return model.Job{}, fmt.Errorf("feedback receipt has contradictory status %q", receipt.Status)
		}
	case "interrupt":
		expectedStatus = model.JobStatusWaiting
	case "replan":
		expectedStatus = model.JobStatusRunning
	case "cancel":
		expectedStatus = model.JobStatusCanceled
	default:
		return model.Job{}, fmt.Errorf("unsupported lifecycle receipt action %q", action)
	}
	if expectedStatus != "" && receipt.Status != expectedStatus {
		return model.Job{}, fmt.Errorf(
			"%s receipt has status %q, expected %q",
			action,
			receipt.Status,
			expectedStatus,
		)
	}
	return model.Job{ID: receipt.JobID, Status: receipt.Status}, nil
}

func validateLifecycleControl(
	channel model.Channel,
	workspaceIdentity string,
	jobID int64,
	operationID queue.LifecycleOperationID,
	text, name string,
	maximum int,
) error {
	if err := validateLifecycleWorkspace(channel, workspaceIdentity); err != nil {
		return err
	}
	if jobID < 1 {
		return fmt.Errorf("%s requires a positive job ID", name)
	}
	if _, err := queue.ParseLifecycleOperationID(string(operationID)); err != nil {
		return err
	}
	if !utf8.ValidString(text) || strings.ContainsRune(text, '\x00') {
		return fmt.Errorf("%s must be valid UTF-8 without NUL", name)
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("%s is required", name)
	}
	if len(text) > maximum {
		return fmt.Errorf("%s exceeds the %d-byte bound", name, maximum)
	}
	return nil
}

func validateLifecycleWorkspace(channel model.Channel, workspaceIdentity string) error {
	if err := channel.ValidateStored(); err != nil {
		return fmt.Errorf("lifecycle channel authority: %w", err)
	}
	if channel.Scope != model.ChannelScopeUser || channel.Mode != model.ChannelModeAssistant {
		return fmt.Errorf("lifecycle control requires one assistant user channel")
	}
	if err := projectroot.ValidateDirectoryIdentity(workspaceIdentity); err != nil {
		return fmt.Errorf("lifecycle workspace identity: %w", err)
	}
	return nil
}

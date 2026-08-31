package client

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
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
	jobID int64,
	operationID queue.LifecycleOperationID,
	feedback string,
) (model.Job, error) {
	return client.feedbackControl(ctx, jobID, operationID, "feedback", feedback, maxCancelReasonBytes)
}

func (client *Client) Interrupt(
	ctx context.Context,
	jobID int64,
	operationID queue.LifecycleOperationID,
	feedback string,
) (model.Job, error) {
	return client.feedbackControl(ctx, jobID, operationID, "interrupt", feedback, maxObjectiveControlBytes)
}

func (client *Client) Replan(
	ctx context.Context,
	jobID int64,
	operationID queue.LifecycleOperationID,
	feedback string,
) (model.Job, error) {
	return client.feedbackControl(ctx, jobID, operationID, "replan", feedback, maxObjectiveControlBytes)
}

func (client *Client) Cancel(
	ctx context.Context,
	jobID int64,
	operationID queue.LifecycleOperationID,
	reason string,
) (model.Job, error) {
	if err := validateLifecycleControl(jobID, operationID, reason, "cancel reason", maxCancelReasonBytes); err != nil {
		return model.Job{}, err
	}
	return client.postLifecycleControl(
		ctx, jobID, operationID, "cancel",
		struct {
			OperationID queue.LifecycleOperationID `json:"operation_id"`
			Reason      string                     `json:"reason"`
		}{OperationID: operationID, Reason: reason},
	)
}

func (client *Client) feedbackControl(
	ctx context.Context,
	jobID int64,
	operationID queue.LifecycleOperationID,
	action, feedback string,
	maximum int,
) (model.Job, error) {
	if err := validateLifecycleControl(
		jobID, operationID, feedback, action+" feedback", maximum,
	); err != nil {
		return model.Job{}, err
	}
	return client.postLifecycleControl(
		ctx, jobID, operationID, action,
		struct {
			OperationID queue.LifecycleOperationID `json:"operation_id"`
			Feedback    string                     `json:"feedback"`
		}{OperationID: operationID, Feedback: feedback},
	)
}

func (client *Client) postLifecycleControl(
	ctx context.Context,
	jobID int64,
	operationID queue.LifecycleOperationID,
	action string,
	payload any,
) (model.Job, error) {
	var receipt lifecycleControlReceipt
	path := fmt.Sprintf("/v1/jobs/%d/%s", jobID, action)
	if err := client.doJSON(ctx, http.MethodPost, path, payload, &receipt); err != nil {
		return model.Job{}, err
	}
	if receipt.JobID != jobID || receipt.OperationID != operationID {
		return model.Job{}, fmt.Errorf("%s receipt does not match the exact submitted job operation", action)
	}
	switch receipt.Status {
	case model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting,
		model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCanceled:
	default:
		return model.Job{}, fmt.Errorf("%s receipt contains unsupported status %q", action, receipt.Status)
	}
	return model.Job{ID: receipt.JobID, Status: receipt.Status}, nil
}

func validateLifecycleControl(
	jobID int64,
	operationID queue.LifecycleOperationID,
	text, name string,
	maximum int,
) error {
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

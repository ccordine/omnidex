package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

type lifecycleControlReceipt struct {
	JobID       int64                      `json:"job_id"`
	OperationID queue.LifecycleOperationID `json:"operation_id"`
	Status      string                     `json:"status"`
}

func (receipt *lifecycleControlReceipt) UnmarshalJSON(raw []byte) error {
	type wireReceipt lifecycleControlReceipt
	var wire wireReceipt
	if err := exactjson.ValidateObject(raw, &wire, "lifecycle control receipt"); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return fmt.Errorf("decode lifecycle control receipt: %w", err)
	}
	*receipt = lifecycleControlReceipt(wire)
	return nil
}

func (receipt lifecycleControlReceipt) job(expectedID int64, expectedOperationID queue.LifecycleOperationID) (model.Job, error) {
	if receipt.JobID != expectedID {
		return model.Job{}, fmt.Errorf("lifecycle control expected job %d, server returned job %d", expectedID, receipt.JobID)
	}
	parsedOperationID, err := queue.ParseLifecycleOperationID(string(receipt.OperationID))
	if err != nil || parsedOperationID != expectedOperationID {
		return model.Job{}, fmt.Errorf("lifecycle control response does not attest the submitted operation")
	}
	switch receipt.Status {
	case model.JobStatusPending, model.JobStatusRunning, model.JobStatusCompleted,
		model.JobStatusFailed, model.JobStatusCanceled, model.JobStatusWaiting:
	default:
		return model.Job{}, fmt.Errorf("lifecycle control response contains unregistered job status %q", receipt.Status)
	}
	return model.Job{ID: receipt.JobID, Status: receipt.Status}, nil
}

func (c *Client) postLifecycleControl(
	ctx context.Context,
	path string,
	expectedID int64,
	operationID queue.LifecycleOperationID,
	payload map[string]any,
) (model.Job, error) {
	var receipt lifecycleControlReceipt
	if err := c.doJSON(ctx, http.MethodPost, path, payload, &receipt); err != nil {
		return model.Job{}, err
	}
	return receipt.job(expectedID, operationID)
}

func (c *Client) SubmitFeedback(ctx context.Context, id int64, operationID queue.LifecycleOperationID, feedback string) (model.Job, error) {
	return c.postLifecycleControl(ctx, fmt.Sprintf("/v1/jobs/%d/feedback", id), id, operationID, map[string]any{
		"operation_id": operationID, "feedback": feedback,
	})
}

func (c *Client) Interrupt(ctx context.Context, id int64, operationID queue.LifecycleOperationID, feedback string) (model.Job, error) {
	return c.postLifecycleControl(ctx, fmt.Sprintf("/v1/jobs/%d/interrupt", id), id, operationID, map[string]any{
		"operation_id": operationID, "feedback": feedback,
	})
}

func (c *Client) Cancel(ctx context.Context, command queue.CancelJobCommand) (model.Job, error) {
	return c.postLifecycleControl(ctx, fmt.Sprintf("/v1/jobs/%d/cancel", command.JobID), command.JobID, command.OperationID, map[string]any{
		"operation_id": command.OperationID, "reason": command.Reason,
	})
}

func (c *Client) Replan(ctx context.Context, id int64, operationID queue.LifecycleOperationID, feedback string) (model.Job, error) {
	return c.postLifecycleControl(ctx, fmt.Sprintf("/v1/jobs/%d/replan", id), id, operationID, map[string]any{
		"operation_id": operationID, "feedback": feedback,
	})
}

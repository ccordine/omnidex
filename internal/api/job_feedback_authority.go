package api

import (
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

type lifecycleControlReceipt struct {
	JobID       int64                      `json:"job_id"`
	OperationID queue.LifecycleOperationID `json:"operation_id"`
	Status      string                     `json:"status"`
}

func validateSameJobAuthority(expectedID int64, job model.Job) error {
	if expectedID <= 0 {
		return fmt.Errorf("feedback authority requires a positive expected job id")
	}
	if job.ID != expectedID {
		return fmt.Errorf("feedback authority expected job %d, repository returned job %d", expectedID, job.ID)
	}
	return nil
}

func newLifecycleControlReceipt(
	expectedID int64,
	operationID queue.LifecycleOperationID,
	job model.Job,
) (lifecycleControlReceipt, error) {
	if err := validateSameJobAuthority(expectedID, job); err != nil {
		return lifecycleControlReceipt{}, err
	}
	if _, err := queue.ParseLifecycleOperationID(string(operationID)); err != nil {
		return lifecycleControlReceipt{}, err
	}
	switch job.Status {
	case model.JobStatusPending, model.JobStatusRunning, model.JobStatusCompleted,
		model.JobStatusFailed, model.JobStatusCanceled, model.JobStatusWaiting:
	default:
		return lifecycleControlReceipt{}, fmt.Errorf("job %d returned unregistered status %q", job.ID, job.Status)
	}
	return lifecycleControlReceipt{JobID: job.ID, OperationID: operationID, Status: job.Status}, nil
}

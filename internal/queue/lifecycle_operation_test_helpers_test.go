package queue

import (
	"strconv"
	"testing"
)

func testLifecycleOperationID(t *testing.T, label string, ownerID int64) LifecycleOperationID {
	t.Helper()
	id, err := NewLifecycleOperationID("queue-test", label, strconv.FormatInt(ownerID, 10))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func testReplanCommand(t *testing.T, jobID int64, label, feedback string) ReplanJobCommand {
	t.Helper()
	return ReplanJobCommand{
		OperationID: testLifecycleOperationID(t, label, jobID),
		JobID:       jobID,
		Feedback:    feedback,
	}
}

func testCancelCommand(t *testing.T, jobID int64, label, reason string) CancelJobCommand {
	t.Helper()
	return CancelJobCommand{
		OperationID: testLifecycleOperationID(t, label, jobID),
		JobID:       jobID,
		Reason:      reason,
	}
}

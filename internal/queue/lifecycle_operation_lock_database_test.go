package queue

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresCrossKindLifecycleIdentitySerializesBeforeAggregateLocks(t *testing.T) {
	repository, pool, baseContext := replanTestRepository(t)
	ctx, cancel := context.WithTimeout(baseContext, 10*time.Second)
	defer cancel()
	job, err := repository.EnqueueJob(
		ctx, fmt.Sprintf("lifecycle-cross-kind-%d", time.Now().UnixNano()),
		model.PipelineCoding, []byte(`{}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	operationID := testLifecycleOperationID(t, "cancel-vs-replan", job.ID)
	errorsOut := make(chan error, 2)
	var start sync.WaitGroup
	start.Add(1)
	go func() {
		start.Wait()
		_, callErr := repository.CancelJob(ctx, CancelJobCommand{
			OperationID: operationID, JobID: job.ID, Reason: "cross-kind cancel",
		})
		errorsOut <- callErr
	}()
	go func() {
		start.Wait()
		_, callErr := repository.ReplanJob(ctx, ReplanJobCommand{
			OperationID: operationID, JobID: job.ID, Feedback: "cross-kind replan",
		})
		errorsOut <- callErr
	}()
	start.Done()

	var successes, conflicts int
	for range 2 {
		callErr := <-errorsOut
		switch {
		case callErr == nil:
			successes++
		case errors.Is(callErr, ErrLifecycleOperationConflict):
			conflicts++
		default:
			t.Fatalf("cross-kind lifecycle result error=%v", callErr)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("cross-kind results successes=%d conflicts=%d", successes, conflicts)
	}
	var registryRows, operationRows int
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM lifecycle_operation_registry WHERE operation_id=$1),
			(SELECT COUNT(*) FROM job_lifecycle_operations WHERE operation_id=$1)
	`, operationID).Scan(&registryRows, &operationRows); err != nil {
		t.Fatal(err)
	}
	if registryRows != 1 || operationRows != 1 {
		t.Fatalf("cross-kind receipts registry=%d operation=%d", registryRows, operationRows)
	}
	cleanup := testCancelCommand(t, job.ID, "cross-kind-cleanup", "close cross-kind fixture")
	if _, err := repository.CancelJob(ctx, cleanup); err != nil && !errors.Is(err, ErrStepNotWritable) {
		t.Fatalf("clean up cross-kind fixture: %v", err)
	}
}

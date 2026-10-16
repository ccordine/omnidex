package codingagent

import (
	"context"
	"errors"
	"testing"
)

func TestInterruptedTaskCannotReplayWhileContinuationMinificationIsUnavailable(t *testing.T) {
	executions := 0
	controller, journal := fixtureController(t, fixtureWorkload(), func(call Assignment) (any, error) {
		executions++
		return nil, &IncompleteTaskError{Reason: "native context exhausted", ThreadID: "original-thread"}
	}, passingChecks)
	task := fixtureWorkload().Tasks[0]
	for range 5 {
		var incomplete *IncompleteTaskError
		if err := controller.RunTask(context.Background(), task); !errors.As(err, &incomplete) {
			t.Fatalf("interruption lost: %v", err)
		}
	}
	if executions != 1 {
		t.Fatalf("interrupted task was replayed %d times", executions)
	}
	count := 0
	for _, entry := range journal.entries {
		if entry.Phase == "task_interrupted" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("durable handoff duplicated: %d", count)
	}
}

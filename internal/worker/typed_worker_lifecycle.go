package worker

import (
	"context"
	"errors"

	"github.com/gryph/omnidex/internal/queue"
)

func emitTypedWorkerFailure(runtime typedWorkerRuntime, event typedWorkerEvent, err error) {
	event.State = typedWorkerFailed
	if typedWorkerFailureIsStop(runtime, err) {
		event.State = typedWorkerStopped
	} else {
		event.Detail = trimForBudget(err.Error(), 1200)
	}
	emitTypedWorker(runtime, event)
}

func typedWorkerFailureIsStop(runtime typedWorkerRuntime, err error) bool {
	contextCanceled := runtime.Context != nil && errors.Is(runtime.Context.Err(), context.Canceled)
	return expectedWorkerStopCause(err, contextCanceled)
}

// Joined failures stop quietly only when every cause belongs to the ended
// execution. An unrelated provider, validation or persistence failure remains
// a failure even if another cause records revoked authority.
func expectedWorkerStopCause(err error, contextCanceled bool) bool {
	if err == nil {
		return false
	}
	switch wrapped := err.(type) {
	case interface{ Unwrap() []error }:
		causes := wrapped.Unwrap()
		if len(causes) == 0 {
			return false
		}
		for _, cause := range causes {
			if !expectedWorkerStopCause(cause, contextCanceled) {
				return false
			}
		}
		return true
	case interface{ Unwrap() error }:
		return expectedWorkerStopCause(wrapped.Unwrap(), contextCanceled)
	default:
		return errors.Is(err, queue.ErrStaleStepAttempt) ||
			errors.Is(err, queue.ErrLLMCallTerminalizedByAttempt) ||
			contextCanceled && errors.Is(err, context.Canceled)
	}
}

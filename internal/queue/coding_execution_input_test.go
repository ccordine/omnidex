package queue

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestNativeInputIdentityCannotBeUsedForUnrelatedFeedback(t *testing.T) {
	id := int64(1)
	_, _, err := applyJobFeedbackTx(context.Background(), nil, SubmitJobFeedbackCommand{ExpectedInputID: &id}, model.Job{Metadata: []byte(`{}`)})
	if !errors.Is(err, ErrStepNotWritable) {
		t.Fatalf("native input identity was ignored outside its owning workflow: %v", err)
	}
}

func TestExecutionReplyDistinguishesUnwritableStateFromStaleGeneration(t *testing.T) {
	for _, status := range []string{model.JobStatusPending, model.JobStatusRunning, model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCanceled} {
		t.Run(status, func(t *testing.T) {
			generation := int64(2)
			job := model.Job{Status: status, CurrentGeneration: generation}
			_, _, err := resumeCodingExecutionTx(context.Background(), nil, SubmitJobFeedbackCommand{ExpectedGeneration: &generation}, job)
			if !errors.Is(err, ErrStepNotWritable) || errors.Is(err, ErrJobFeedbackGeneration) {
				t.Fatalf("non-waiting execution reply = %v, want unwritable state", err)
			}
		})
	}
	stale := int64(1)
	job := model.Job{Status: model.JobStatusWaiting, CurrentGeneration: 2}
	_, _, err := resumeCodingExecutionTx(context.Background(), nil, SubmitJobFeedbackCommand{ExpectedGeneration: &stale}, job)
	if !errors.Is(err, ErrJobFeedbackGeneration) || errors.Is(err, ErrStepNotWritable) {
		t.Fatalf("stale execution reply = %v, want stale generation", err)
	}
}

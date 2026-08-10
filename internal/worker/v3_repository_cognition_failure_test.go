package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/queue"
)

func TestRepositoryCognitionCancellationIsClosedAndTyped(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name string
		err  error
		code cognitionruntime.CancellationCode
	}{
		{"invalid_decision", fmtWrap(cognitionpolicy.ErrInvalidDecision), cognitionruntime.CancellationPolicyFailure},
		{"response_limit", fmtWrap(cognitionpolicy.ErrResponseLimit), cognitionruntime.CancellationPolicyFailure},
		{"generation", fmtWrap(cognitionpolicy.ErrGeneration), cognitionruntime.CancellationPolicyFailure},
		{"coordinator_budget", fmtWrap(cognition.ErrCoordinatorBudgetExhausted), cognitionruntime.CancellationRunBudgetExhausted},
		{"cycle_budget", fmtWrap(cognitionruntime.ErrRunCycleLimit), cognitionruntime.CancellationRunBudgetExhausted},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			code, message, allowed := repositoryCognitionCancellation(testCase.err)
			if !allowed || code != testCase.code || message == "" {
				t.Fatalf("classification=%q/%q/%v", code, message, allowed)
			}
		})
	}

	for _, forbidden := range []error{
		nil,
		context.Canceled,
		cognitionpolicy.ErrProviderIdentity,
		errors.Join(cognitionpolicy.ErrGeneration, cognitionpolicy.ErrProviderIdentity),
		cognitionpolicy.ErrInvalidBrain,
		cognitionpolicy.ErrCallJournal,
		cognitionpolicy.ErrCallIndeterminate,
		errors.Join(cognitionpolicy.ErrInvalidDecision, cognitionpolicy.ErrCallJournal),
		errors.Join(cognitionpolicy.ErrResponseLimit, cognitionpolicy.ErrInvalidEvidence),
		errors.Join(cognitionpolicy.ErrGeneration, cognitionpolicy.ErrInvalidBrain),
		errors.Join(cognitionpolicy.ErrGeneration, cognition.ErrEnvironmentJournalConflict),
		errors.Join(cognitionpolicy.ErrInvalidDecision, queue.ErrCognitionConflict),
		cognitionruntime.ErrInvalidJournalState,
		cognition.ErrEnvironmentJournalConflict,
		queue.ErrCognitionConflict,
		errors.New("unknown cognition failure"),
	} {
		if code, message, allowed := repositoryCognitionCancellation(forbidden); allowed || code != "" || message != "" {
			t.Fatalf("forbidden error %v classified as %q/%q", forbidden, code, message)
		}
	}
}

func fmtWrap(err error) error {
	return &repositoryCognitionWrappedError{cause: err}
}

type repositoryCognitionWrappedError struct{ cause error }

func (value *repositoryCognitionWrappedError) Error() string {
	return "wrapped: " + value.cause.Error()
}
func (value *repositoryCognitionWrappedError) Unwrap() error { return value.cause }

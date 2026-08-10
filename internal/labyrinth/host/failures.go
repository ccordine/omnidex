package host

import (
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/labyrinth"
)

func episodeLimitFailure(
	scenario labyrinth.Scenario,
	episode storedEpisode,
	expected cognition.WorldRevision,
	action cognition.RegisteredAction,
) error {
	schema, exists := scenario.Catalog().Schema(action.Request.Kind)
	if !exists || action.Validate(schema) != nil {
		return publicFailure(
			cognition.ActionFailureInvalidAction, action, expected,
			"The registered action does not match the public action catalog.", cognition.ErrInvalidAction,
		)
	}
	if expected != episode.Current {
		return publicFailure(
			cognition.ActionFailureStaleRevision, action, expected,
			"The expected world revision is not current.", cognition.ErrInvalidRevision,
		)
	}
	if episode.Terminal {
		return publicFailure(
			cognition.ActionFailureTerminal, action, expected,
			"The environment has already reached its terminal predicate.", labyrinth.ErrTerminal,
		)
	}
	return publicFailure(
		cognition.ActionFailureBudget, action, expected,
		"The episode transition budget is exhausted.", labyrinth.ErrTransitionLimit,
	)
}

func publicFailure(
	code cognition.ActionFailureCode,
	action cognition.RegisteredAction,
	expected cognition.WorldRevision,
	message string,
	cause error,
) error {
	failure, err := cognition.NewActionFailure(code, action, expected, message, nil)
	if err != nil {
		return fmt.Errorf("%w: construct durable host failure: %v", cause, err)
	}
	return errors.Join(failure, cause)
}

func receiptFailure(receipt ActionReceipt) error {
	if receipt.Failure == nil {
		return fmt.Errorf("%w: failure receipt has no failure", ErrReceiptCorrupt)
	}
	failure := receipt.Failure.Clone()
	var cause error
	switch failure.Code {
	case cognition.ActionFailureInvalidAction:
		cause = cognition.ErrInvalidAction
	case cognition.ActionFailurePreconditionFailed:
		cause = labyrinth.ErrPrecondition
	case cognition.ActionFailureUnauthorized:
		cause = cognition.ErrAuthorityDenied
	case cognition.ActionFailureStaleRevision:
		cause = cognition.ErrInvalidRevision
	case cognition.ActionFailureIdempotencyConflict:
		cause = labyrinth.ErrReplayConflict
	case cognition.ActionFailureTerminal:
		cause = labyrinth.ErrTerminal
	case cognition.ActionFailureBudget:
		cause = labyrinth.ErrTransitionLimit
	default:
		return fmt.Errorf("%w: unregistered failure code %q", ErrReceiptCorrupt, failure.Code)
	}
	return errors.Join(failure, cause)
}

func receiptResult(receipt ActionReceipt) (cognition.Transition, error) {
	if receipt.Transition != nil && receipt.Failure == nil {
		return receipt.Transition.Clone(), nil
	}
	if receipt.Failure != nil && receipt.Transition == nil {
		return cognition.Transition{}, receiptFailure(receipt)
	}
	return cognition.Transition{}, fmt.Errorf("%w: receipt must contain exactly one result", ErrReceiptCorrupt)
}

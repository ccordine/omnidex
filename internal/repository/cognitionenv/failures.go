package cognitionenv

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

func (environment *Environment) failure(
	action cognition.RegisteredAction,
	expected cognition.WorldRevision,
	code cognition.ActionFailureCode,
	message string,
) error {
	failure, err := environment.actionFailure(action, expected, code, message)
	if err != nil {
		return err
	}
	return failure
}

func (environment *Environment) actionFailure(
	action cognition.RegisteredAction,
	expected cognition.WorldRevision,
	code cognition.ActionFailureCode,
	message string,
) (cognition.ActionFailure, error) {
	return cognition.NewActionFailure(code, action, expected, message, nil)
}

func (environment *Environment) commitFailure(
	ctx context.Context,
	action cognition.RegisteredAction,
	expected cognition.WorldRevision,
	code cognition.ActionFailureCode,
	message string,
) (cognition.Transition, error) {
	failure, err := environment.actionFailure(action, expected, code, message)
	if err != nil {
		return cognition.Transition{}, err
	}
	receipt, err := cognition.NewEnvironmentFailureReceipt(
		environment.episode, action, expected, failure,
	)
	if err != nil {
		return cognition.Transition{}, err
	}
	receipt, err = environment.journal.CommitEnvironmentAction(
		ctx, environment.episode, environment.investigation.ref, receipt,
	)
	if err != nil {
		return cognition.Transition{}, environment.journalActionError(action, expected, err)
	}
	return environment.resultFromReceipt(receipt)
}

func (environment *Environment) resultFromReceipt(
	receipt cognition.EnvironmentReceipt,
) (cognition.Transition, error) {
	if err := receipt.Validate(environment.episode); err != nil {
		return cognition.Transition{}, fmt.Errorf("repository cognition journal receipt: %w", err)
	}
	if receipt.Transition != nil {
		return receipt.Transition.Clone(), nil
	}
	return cognition.Transition{}, receipt.Failure.Clone()
}

func (environment *Environment) journalActionError(
	action cognition.RegisteredAction,
	expected cognition.WorldRevision,
	err error,
) error {
	switch {
	case errors.Is(err, cognition.ErrEnvironmentJournalConflict):
		return environment.failure(action, expected, cognition.ActionFailureIdempotencyConflict,
			"action identity was already bound to different content")
	case errors.Is(err, cognition.ErrEnvironmentJournalStaleRevision):
		return environment.failure(action, expected, cognition.ActionFailureStaleRevision,
			"expected repository revision is stale")
	case errors.Is(err, cognition.ErrEnvironmentJournalTerminal):
		return environment.failure(action, expected, cognition.ActionFailureTerminal,
			"repository investigation is already terminal")
	default:
		return fmt.Errorf("repository cognition environment journal: %w", err)
	}
}

package host

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/labyrinth"
	"github.com/jackc/pgx/v5"
)

func (environment *Environment) Apply(
	ctx context.Context,
	episode cognition.EpisodeRef,
	expected cognition.WorldRevision,
	action cognition.RegisteredAction,
) (cognition.Transition, error) {
	if err := environment.validate(ctx); err != nil {
		return cognition.Transition{}, err
	}
	if err := ctx.Err(); err != nil {
		return cognition.Transition{}, err
	}
	if err := action.Actor.Validate(); err != nil {
		return cognition.Transition{}, fmt.Errorf("%w: %v", cognition.ErrInvalidAction, err)
	}
	if err := environment.authorize(ctx, action.Actor); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return cognition.Transition{}, contextErr
		}
		return cognition.Transition{}, publicFailure(
			cognition.ActionFailureUnauthorized, action, expected,
			"The current execution attempt is not authorized.", cognition.ErrAuthorityDenied,
		)
	}
	if err := episode.Validate(); err != nil || episode != environment.episode {
		return cognition.Transition{}, publicFailure(
			cognition.ActionFailureStaleRevision, action, expected,
			"The requested episode is not current.", cognition.ErrInvalidRevision,
		)
	}
	if err := expected.Validate(); err != nil {
		return cognition.Transition{}, err
	}
	requestDigest, err := actionRequestSHA256(action)
	if err != nil {
		return cognition.Transition{}, err
	}
	tx, err := environment.store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return cognition.Transition{}, err
	}
	defer tx.Rollback(ctx)
	if err := environment.authorizeMutation(ctx, tx, action, expected); err != nil {
		return cognition.Transition{}, err
	}
	stored, err := loadEpisodeRow(ctx, tx, environment.store.schema, episode, true)
	if err != nil {
		return cognition.Transition{}, err
	}
	if existing, found, loadErr := loadActionRow(ctx, tx, environment.store.schema, episode, action.ID); loadErr != nil {
		return cognition.Transition{}, loadErr
	} else if found {
		if existing.Receipt.RequestSHA256 != requestDigest || existing.Receipt.Expected != expected {
			return cognition.Transition{}, publicFailure(
				cognition.ActionFailureIdempotencyConflict, action, expected,
				"The action identity is already bound to a different request.", labyrinth.ErrReplayConflict,
			)
		}
		if err := tx.Commit(ctx); err != nil {
			return cognition.Transition{}, fmt.Errorf("commit Labyrinth receipt replay: %w", err)
		}
		return receiptResult(existing.Receipt)
	}
	scenario, err := environment.resolveScenario(ctx, stored.Scenario)
	if err != nil {
		return cognition.Transition{}, err
	}
	if stored.ReceiptCount >= labyrinth.MaxEpisodeTransitions {
		return cognition.Transition{}, episodeLimitFailure(scenario, stored, expected, action)
	}
	history, err := loadSuccessfulHistory(ctx, tx, environment.store.schema, episode)
	if err != nil {
		return cognition.Transition{}, err
	}
	candidate, err := reconstructCandidate(ctx, environment, scenario, stored, history)
	if err != nil {
		return cognition.Transition{}, err
	}
	transition, applyErr := candidate.Apply(ctx, episode, expected, action)
	if closeErr := candidate.Close(); closeErr != nil {
		return cognition.Transition{}, fmt.Errorf("close Labyrinth action kernel: %w", closeErr)
	}
	receipt := ActionReceipt{
		Episode: episode, Action: action.Clone(), Expected: expected, RequestSHA256: requestDigest,
	}
	if applyErr == nil {
		if err := transition.ValidateApply(episode, expected, action); err != nil {
			return cognition.Transition{}, fmt.Errorf("%w: candidate transition: %v", ErrReceiptCorrupt, err)
		}
		receipt.Transition = pointerTransition(transition)
	} else {
		var failure cognition.ActionFailure
		if !errors.As(applyErr, &failure) {
			return cognition.Transition{}, applyErr
		}
		if err := failure.Validate(action, expected); err != nil {
			return cognition.Transition{}, fmt.Errorf("%w: candidate failure: %v", ErrReceiptCorrupt, err)
		}
		receipt.Failure = pointerFailure(failure)
	}
	if err := environment.authorizeMutation(ctx, tx, action, expected); err != nil {
		return cognition.Transition{}, err
	}
	if err := persistActionReceipt(ctx, tx, environment.store.schema, stored, receipt); err != nil {
		return cognition.Transition{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return cognition.Transition{}, fmt.Errorf("commit Labyrinth action receipt: %w", err)
	}
	if applyErr != nil {
		return cognition.Transition{}, receiptFailure(receipt)
	}
	return transition.Clone(), nil
}

func (environment *Environment) authorizeMutation(
	ctx context.Context,
	tx pgx.Tx,
	action cognition.RegisteredAction,
	expected cognition.WorldRevision,
) error {
	if err := environment.authorizeTransaction(ctx, tx, action.Actor); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		return publicFailure(
			cognition.ActionFailureUnauthorized, action, expected,
			"The current execution attempt is not authorized.", cognition.ErrAuthorityDenied,
		)
	}
	return nil
}

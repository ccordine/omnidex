package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

// AuthorizeStepAttempt verifies the exact current lease and running job/step
// state without granting mutation authority or returning internal queue state.
func (r *Repository) AuthorizeStepAttempt(
	ctx context.Context,
	authority model.StepAttemptAuthority,
) error {
	if ctx == nil || r == nil || r.pool == nil {
		return fmt.Errorf("step-attempt authorization requires PostgreSQL and context")
	}
	if err := validateStepAttemptAuthority(authority); err != nil {
		return err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := r.AuthorizeStepAttemptTransaction(ctx, tx, authority); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// AuthorizeStepAttemptTransaction verifies the exact lease while holding the
// caller's mutation transaction. Environment adapters must use this boundary
// immediately before reading or changing environment state; a prior standalone
// authorization is not mutation authority.
func (r *Repository) AuthorizeStepAttemptTransaction(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
) error {
	if ctx == nil || r == nil || r.pool == nil || tx == nil {
		return fmt.Errorf("transactional step-attempt authorization requires PostgreSQL and context")
	}
	if err := validateStepAttemptAuthority(authority); err != nil {
		return err
	}
	schema, err := resolveStepAttemptFenceTransaction(ctx, tx)
	if err != nil {
		return err
	}
	return callStepAttemptFenceTransaction(ctx, tx, schema, authority)
}

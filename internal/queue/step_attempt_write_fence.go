package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

// underActiveStepAttemptFence holds the exact job -> step -> attempt locks while
// a legacy human/code-authority repository operation performs a worker write.
// The guarded operation must not acquire the fenced parent job row itself.
func underActiveStepAttemptFence[T any](
	ctx context.Context,
	repository *Repository,
	authority model.StepAttemptAuthority,
	operation string,
	write func() (T, error),
) (T, error) {
	var zero T
	if repository == nil || repository.pool == nil {
		return zero, fmt.Errorf("%s: PostgreSQL repository is unavailable", operation)
	}
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return zero, err
	}
	released := false
	defer func() {
		if !released {
			_ = tx.Rollback(context.Background())
		}
	}()
	jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, authority)
	if err != nil {
		return zero, err
	}
	if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
		return zero, staleStepAttemptError(authority, operation+" requires a running job and step", nil)
	}
	result, writeErr := write()
	releaseErr := tx.Rollback(context.Background())
	released = true
	if releaseErr != nil && !errors.Is(releaseErr, pgx.ErrTxClosed) {
		releaseErr = fmt.Errorf("release %s attempt fence: %w", operation, releaseErr)
	} else {
		releaseErr = nil
	}
	if writeErr != nil || releaseErr != nil {
		return zero, errors.Join(writeErr, releaseErr)
	}
	return result, nil
}

func underActiveStepAttemptWriteFence(
	ctx context.Context,
	repository *Repository,
	authority model.StepAttemptAuthority,
	operation string,
	write func() error,
) error {
	_, err := underActiveStepAttemptFence(
		ctx, repository, authority, operation,
		func() (struct{}, error) { return struct{}{}, write() },
	)
	return err
}

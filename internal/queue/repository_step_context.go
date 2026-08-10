package queue

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const (
	maxStepContextKeyBytes   = 512
	maxStepContextValueBytes = 1 << 20
)

func (r *Repository) AddStepContext(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	key, value string,
) error {
	key = strings.TrimSpace(key)
	if key == "" || len(key) > maxStepContextKeyBytes || !utf8.ValidString(key) || strings.ContainsRune(key, '\x00') {
		return fmt.Errorf("step context key must be exact PostgreSQL-compatible text of at most %d bytes", maxStepContextKeyBytes)
	}
	if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') || len(value) > maxStepContextValueBytes {
		return fmt.Errorf("step context value must be PostgreSQL-compatible text of at most %d bytes", maxStepContextValueBytes)
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, authority)
	if err != nil {
		return err
	}
	if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
		return staleStepAttemptError(authority, "step-context writer is not running", nil)
	}
	result, err := tx.Exec(ctx, `
		WITH inserted AS (
			INSERT INTO step_contexts (step_id, key, value)
			SELECT steps.id, $2, $3
			FROM job_steps AS steps
			JOIN jobs ON jobs.id=steps.job_id
			WHERE steps.id=$1
			  AND steps.job_id=$4
			  AND steps.superseded_at_generation IS NULL
			  AND steps.generation=jobs.current_generation
			RETURNING step_id
		)
		UPDATE job_steps
		SET updated_at = NOW()
		WHERE id = (SELECT step_id FROM inserted)
	`, authority.StepID, key, value, authority.JobID)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return staleStepAttemptError(authority, "step-context target lost current authority", nil)
	}
	return tx.Commit(ctx)
}

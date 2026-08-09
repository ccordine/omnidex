package queue

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

const (
	maxStepContextKeyBytes   = 512
	maxStepContextValueBytes = 1 << 20
)

func (r *Repository) AddStepContext(ctx context.Context, stepID int64, key, value string) error {
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
	jobID, err := stepJobIDTx(ctx, tx, stepID)
	if err != nil {
		return err
	}
	if err := requireRunningCurrentStepTx(ctx, tx, jobID, stepID); err != nil {
		return err
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
	`, stepID, key, value, jobID)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("%w: step context target %d lost current authority", ErrStaleJobGeneration, stepID)
	}
	return tx.Commit(ctx)
}

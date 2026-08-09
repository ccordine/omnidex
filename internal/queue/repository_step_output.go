package queue

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const maxStepOutputDeltaBytes = 1 << 20

// AppendStepOutput appends one bounded chunk to the current running step. A
// retired worker may retain its step identity for audit, but cannot append to
// the new generation's live output.
func (r *Repository) AppendStepOutput(ctx context.Context, stepID int64, delta string) error {
	if delta == "" {
		return nil
	}
	if !utf8.ValidString(delta) || strings.ContainsRune(delta, '\x00') {
		return fmt.Errorf("step output delta must be PostgreSQL-compatible UTF-8")
	}
	if len(delta) > maxStepOutputDeltaBytes {
		return fmt.Errorf("step output delta has %d bytes; maximum is %d", len(delta), maxStepOutputDeltaBytes)
	}
	if !strings.HasSuffix(delta, "\n") {
		delta += "\n"
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	jobID, jobStatus, err := lockCurrentGenerationStepByID(ctx, tx, stepID)
	if err != nil {
		return err
	}
	if jobStatus != model.JobStatusRunning {
		return fmt.Errorf(
			"%w: job %d status is %q, expected %q",
			ErrStepNotWritable, jobID, jobStatus, model.JobStatusRunning,
		)
	}
	result, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET output = COALESCE(output, '') || $2,
		    updated_at = NOW()
		WHERE id = $1
		  AND status = $3
		  AND superseded_at_generation IS NULL
	`, stepID, delta, model.StepStatusRunning)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("%w: step %d is not running", ErrStepNotWritable, stepID)
	}
	return tx.Commit(ctx)
}

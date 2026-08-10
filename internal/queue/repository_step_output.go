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
func (r *Repository) AppendStepOutput(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	delta string,
) error {
	if err := validateStepAttemptAuthority(authority); err != nil {
		return err
	}
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
	jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, authority)
	if err != nil {
		return err
	}
	if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
		return staleStepAttemptError(authority, fmt.Sprintf(
			"output writer job status %q step status %q", jobStatus, stepStatus,
		), nil)
	}
	result, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET output = COALESCE(output, '') || $2,
		    updated_at = NOW()
		WHERE id = $1 AND job_id=$4 AND generation=$5 AND current_attempt=$6
		  AND worker_id=$7 AND status = $3
		  AND superseded_at_generation IS NULL
	`, authority.StepID, delta, model.StepStatusRunning, authority.JobID,
		authority.Generation, authority.Attempt, authority.WorkerID)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return staleStepAttemptError(authority, "output target lost current authority", nil)
	}
	return tx.Commit(ctx)
}

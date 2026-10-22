package queue

import (
	"context"
	"errors"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

// Recovery requires either retained frozen work or the untouched planning
// boundary installed by an owner interruption. Waiting for plan approval alone
// is not recoverable execution authority.
func jobRecoveryStepTx(ctx context.Context, tx pgx.Tx, job model.Job) (stepID int64, planning bool, err error) {
	stepID, err = codingRecoveryStepTx(ctx, tx, job)
	if !errors.Is(err, pgx.ErrNoRows) {
		return stepID, false, err
	}
	err = tx.QueryRow(ctx, `SELECT step.id FROM job_steps step
 WHERE step.job_id=$1 AND step.generation=$2 AND $3='waiting_input'
 AND step.action='v3_coding_plan' AND step.status='waiting_input'
 AND step.current_attempt=0 AND step.superseded_at_generation IS NULL
 AND EXISTS(SELECT 1 FROM job_lifecycle_operations operation WHERE operation.job_id=$1
   AND operation.kind='interrupt_job' AND operation.step_id IS NULL
   AND operation.result_generation=$2 AND operation.observed_generation<$2)
 AND NOT EXISTS(SELECT 1 FROM coding_ledgers ledger WHERE ledger.job_id=$1
   AND ledger.state NOT IN ('proposed','revising'))
 AND NOT EXISTS(SELECT 1 FROM jobs newer WHERE (newer.metadata->>'channel_id'=(SELECT metadata->>'channel_id' FROM jobs WHERE id=$1) OR (newer.pipeline='coding' AND newer.metadata->>'client_workspace_identity'=(SELECT metadata->>'client_workspace_identity' FROM jobs WHERE id=$1)))
   AND newer.id>$1)
 ORDER BY step.id DESC LIMIT 1`, job.ID, job.CurrentGeneration, job.Status).Scan(&stepID)
	return stepID, true, err
}

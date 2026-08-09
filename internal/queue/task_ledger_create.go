package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func createTaskLedgerTx(ctx context.Context, tx pgx.Tx, jobID int64, runID string) error {
	if ctx == nil {
		return fmt.Errorf("create task ledger: context is required")
	}
	if tx == nil {
		return fmt.Errorf("create task ledger: transaction is required")
	}
	owner := taskstate.LedgerOwner{
		Kind:  taskstate.OwnerJob,
		JobID: jobID,
		RunID: runID,
	}
	ledgerID, err := taskstate.NewLedgerID(owner)
	if err != nil {
		return fmt.Errorf("create task ledger for job %d: %w", jobID, err)
	}
	ledger, err := taskstate.NewLedger(ledgerID, owner)
	if err != nil {
		return fmt.Errorf("create task ledger %q for job %d: %w", ledgerID, jobID, err)
	}
	if err := taskstate.ValidateMaterializedState(ledger.MaterializedState()); err != nil {
		return fmt.Errorf("create task ledger %q for job %d: %w", ledgerID, jobID, err)
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO task_ledgers (id, job_id, run_id, owner_type, owner_id, version, status)
		SELECT $1, jobs.id, runs.id, $4, jobs.id, 0, $5
		FROM jobs
		JOIN omni_runs runs ON runs.id=$3::uuid
		WHERE jobs.id=$2
		  AND COALESCE(jobs.metadata ->> 'telemetry_run_id', '')=$3::text
	`, ledgerID, jobID, runID, taskstate.OwnerJob, taskstate.LedgerActive)
	if err != nil {
		return fmt.Errorf("create task ledger %q for job %d: %w", ledgerID, jobID, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf(
			"create task ledger %q for job %d: authoritative job/run binding matched %d rows; expected 1",
			ledgerID, jobID, tag.RowsAffected(),
		)
	}
	return nil
}

package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

// MarkCodingLedgerReviewReady moves the existing planning step to waiting only
// after the normalized proposal is durable.
func (r *Repository) MarkCodingLedgerReviewReady(ctx context.Context, authority model.StepAttemptAuthority, ledgerID, revision int64) error {
	if r == nil || r.pool == nil || ledgerID <= 0 || revision <= 0 {
		return fmt.Errorf("coding ledger review requires exact authority")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	locked, err := lockStepAttemptAuthorityTx(ctx, tx, authority)
	if err != nil {
		return err
	}
	if err := requireLockedStepAttemptActiveTx(ctx, tx, authority, locked); err != nil {
		return err
	}
	if locked.StepStatus != model.StepStatusRunning || locked.JobStatus != model.JobStatusRunning {
		return staleStepAttemptError(authority, "coding ledger review writer is not running", nil)
	}
	if err := requireNoPendingChannelSessionMessagesTx(ctx, tx, authority.JobID, authority.Generation); err != nil {
		return err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM coding_ledgers ledger
		JOIN coding_ledger_revisions revision ON revision.ledger_id=ledger.id AND revision.revision=$2
		WHERE ledger.id=$1 AND ledger.job_id=$3 AND ledger.current_revision=$2 AND ledger.state='proposed'
	)`, ledgerID, revision, authority.JobID).Scan(&exists); err != nil || !exists {
		if err != nil {
			return err
		}
		return fmt.Errorf("coding ledger proposal changed before review")
	}
	if err := terminalizeStepAttemptTx(ctx, tx, authority, model.StepAttemptCompleted); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `UPDATE job_steps SET status=$5,worker_id=NULL,updated_at=clock_timestamp()
		WHERE id=$1 AND job_id=$2 AND generation=$3 AND current_attempt=$4 AND worker_id=$6 AND action='v3_coding_plan' AND status=$7`,
		authority.StepID, authority.JobID, authority.Generation, authority.Attempt, model.StepStatusWaiting, authority.WorkerID, model.StepStatusRunning)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("coding ledger planning step changed before review")
	}
	jobResult, err := tx.Exec(ctx, `UPDATE jobs SET status=$2,updated_at=clock_timestamp() WHERE id=$1 AND current_generation=$4 AND status=$3`, authority.JobID, model.JobStatusWaiting, model.JobStatusRunning, authority.Generation)
	if err != nil {
		return err
	}
	if jobResult.RowsAffected() != 1 {
		return staleStepAttemptError(authority, "coding ledger job changed before review", nil)
	}
	return tx.Commit(ctx)
}

// CompleteCodingLedgerReview advances the existing queue only after the
// normalized frozen receipt exists. Replays finish an interrupted transition
// without rerunning or replacing the ledger freeze.
func (r *Repository) CompleteCodingLedgerReview(ctx context.Context, jobID, ledgerID, frozenRevision int64) error {
	if r == nil || r.pool == nil || jobID <= 0 || ledgerID <= 0 || frozenRevision <= 0 {
		return fmt.Errorf("coding ledger review completion requires exact authority")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := completeCodingLedgerReviewTx(ctx, tx, jobID, ledgerID, frozenRevision); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func completeCodingLedgerReviewTx(ctx context.Context, tx pgx.Tx, jobID, ledgerID, frozenRevision int64) error {
	var generation, stepID int64
	var jobStatus, stepStatus string
	if err := tx.QueryRow(ctx, `
		SELECT job.current_generation,job.status,step.id,step.status
		FROM jobs job
		JOIN coding_ledgers ledger ON ledger.job_id=job.id
		JOIN job_steps step ON step.job_id=job.id AND step.generation=job.current_generation
		 AND step.action='v3_coding_plan' AND step.superseded_at_generation IS NULL
		WHERE job.id=$1 AND ledger.id=$2 AND ledger.state='frozen' AND ledger.frozen_revision=$3
		FOR UPDATE OF job,step
	`, jobID, ledgerID, frozenRevision).Scan(&generation, &jobStatus, &stepID, &stepStatus); err != nil {
		return err
	}
	if jobStatus == model.JobStatusRunning && stepStatus == model.StepStatusCompleted {
		return nil
	}
	if jobStatus != model.JobStatusWaiting || stepStatus != model.StepStatusWaiting {
		return fmt.Errorf("%w: normalized planning boundary is job=%s step=%s", ErrCodingLedgerState, jobStatus, stepStatus)
	}
	if result, err := tx.Exec(ctx, `UPDATE job_steps SET status=$2,output=$3,finished_at=clock_timestamp(),updated_at=clock_timestamp() WHERE id=$1 AND status=$4`,
		stepID, model.StepStatusCompleted, "coding ledger audited and frozen", model.StepStatusWaiting); err != nil || result.RowsAffected() != 1 {
		if err != nil {
			return err
		}
		return fmt.Errorf("%w: normalized planning step changed", ErrCodingLedgerState)
	}
	if result, err := tx.Exec(ctx, `UPDATE jobs SET status=$2,result=NULL,error=NULL,completed_at=NULL,updated_at=clock_timestamp() WHERE id=$1 AND current_generation=$3 AND status=$4`,
		jobID, model.JobStatusRunning, generation, model.JobStatusWaiting); err != nil || result.RowsAffected() != 1 {
		if err != nil {
			return err
		}
		return fmt.Errorf("%w: normalized planning job changed", ErrCodingLedgerState)
	}
	return nil
}

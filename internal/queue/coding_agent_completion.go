package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (repository *Repository) CodingLedgerCompletionCounts(
	ctx context.Context,
	authority model.StepAttemptAuthority,
) (int, error) {
	if repository == nil || repository.pool == nil {
		return 0, fmt.Errorf("coding ledger completion counts require repository authority")
	}
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, authority)
	if err != nil {
		return 0, err
	}
	if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
		return 0, staleStepAttemptError(authority, "coding ledger completion counts require current execution", nil)
	}
	var action string
	if err := tx.QueryRow(ctx, `
		SELECT action FROM job_steps
		WHERE id=$1 AND job_id=$2 AND generation=$3
	`, authority.StepID, authority.JobID, authority.Generation).Scan(&action); err != nil {
		return 0, err
	}
	if action != "v3_coding" {
		return 0, fmt.Errorf("coding ledger completion counts require the execution step")
	}
	var ledgerID, revision int64
	if err := tx.QueryRow(ctx, `
		SELECT id,frozen_revision
		FROM coding_ledgers
		WHERE job_id=$1 AND state='frozen' AND frozen_revision IS NOT NULL
	`, authority.JobID).Scan(&ledgerID, &revision); err != nil {
		return 0, err
	}
	var taskCount, acceptedCount int
	if err := tx.QueryRow(ctx, `
		SELECT count(*),count(*) FILTER (WHERE state='accepted')
		FROM coding_agent_tasks
		WHERE ledger_id=$1 AND frozen_revision=$2
	`, ledgerID, revision).Scan(&taskCount, &acceptedCount); err != nil {
		return 0, err
	}
	if taskCount < 1 || acceptedCount != taskCount {
		return 0, fmt.Errorf("coding ledger completion counts require every frozen task accepted")
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return taskCount, nil
}

// Completion validates the durable journal inside the same transaction that
// terminalizes the current attempt. Agent claims alone cannot close the job.
func requireCodingAgentCompletionTx(ctx context.Context, tx pgx.Tx, job model.Job, stepID int64) error {
	metadataConfiguration, err := codingagent.ConfigurationFromMetadata(job.Metadata)
	if err != nil {
		return err
	}
	if metadataConfiguration.Mode != codingagent.Agents {
		return nil
	}
	var action string
	if err := tx.QueryRow(ctx, `SELECT action FROM job_steps WHERE id=$1 AND job_id=$2`, stepID, job.ID).Scan(&action); err != nil {
		return err
	}
	if action != "v3_coding" {
		return nil
	}
	var ledgerID, frozenRevision int64
	var ledgerState string
	err = tx.QueryRow(ctx, `
		SELECT id,state,frozen_revision
		FROM coding_ledgers
		WHERE job_id=$1
	`, job.ID).Scan(&ledgerID, &ledgerState, &frozenRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("agent completion requires one frozen PostgreSQL coding ledger")
	}
	if err != nil {
		return err
	}
	if ledgerState != "frozen" || frozenRevision < 1 {
		return fmt.Errorf("agent completion requires the current frozen task ledger")
	}
	return materializeCodingTaskLedgerCompletionTx(ctx, tx, ledgerID, frozenRevision)
}

func materializeCodingTaskLedgerCompletionTx(
	ctx context.Context,
	tx pgx.Tx,
	ledgerID, revision int64,
) error {
	var incomplete bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM coding_agent_tasks
		WHERE ledger_id=$1 AND frozen_revision=$2 AND state<>'accepted'
	)`, ledgerID, revision).Scan(&incomplete); err != nil {
		return err
	}
	if incomplete {
		return fmt.Errorf("coding task ledger completion requires every queued task accepted")
	}
	result, err := tx.Exec(ctx, `
		UPDATE coding_ledgers
		SET state='completed',updated_at=clock_timestamp()
		WHERE id=$1 AND frozen_revision=$2 AND state='frozen'
	`, ledgerID, revision)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("coding task ledger changed before completion")
	}
	return nil
}

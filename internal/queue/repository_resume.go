package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ResumeJob(ctx context.Context, command ResumeJobCommand) (LifecycleJobResult, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return LifecycleJobResult{}, err
	}
	defer tx.Rollback(ctx)
	result, err := resumeJobTx(ctx, tx, command)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}

func resumeJobTx(ctx context.Context, tx pgx.Tx, command ResumeJobCommand) (LifecycleJobResult, error) {
	if command.JobID < 1 || command.ExpectedGeneration < 1 || command.ObservedLedgerRevision < 1 {
		return LifecycleJobResult{}, fmt.Errorf("resume requires exact job, generation and frozen revision")
	}
	if err := validateLifecycleText("resume feedback", command.Feedback, 64*1024, false); err != nil {
		return LifecycleJobResult{}, err
	}
	if err := validateLifecycleWorkspaceBinding(command.WorkspaceRoot, command.WorkspaceIdentity); err != nil {
		return LifecycleJobResult{}, err
	}
	descriptor, err := describeLifecycleOperation(command.OperationID, LifecycleResumeJob, command)
	if err != nil {
		return LifecycleJobResult{}, err
	}
	if err := lockLifecycleOperationIdentityTx(ctx, tx, command.OperationID); err != nil {
		return LifecycleJobResult{}, err
	}
	job, err := lockedJobTx(ctx, tx, command.JobID)
	if err != nil {
		return LifecycleJobResult{}, err
	}
	if err := requireLifecycleWorkspaceAuthority(job, command.WorkspaceRoot, command.WorkspaceIdentity); err != nil {
		return LifecycleJobResult{}, err
	}
	if record, found, err := loadLifecycleOperationTx(ctx, tx, descriptor, job.ID); err != nil {
		return LifecycleJobResult{}, err
	} else if found {
		if job.CurrentGeneration != record.ResultGeneration {
			return LifecycleJobResult{}, ErrStaleJobGeneration
		}
		return LifecycleJobResult{Job: record.ResultJob}, nil
	}
	if job.CurrentGeneration != command.ExpectedGeneration {
		return LifecycleJobResult{}, ErrStaleJobGeneration
	}
	var revision int64
	if err := tx.QueryRow(ctx, `SELECT current_revision FROM coding_ledgers WHERE job_id=$1 AND state='frozen' AND current_revision=frozen_revision FOR UPDATE`, job.ID).Scan(&revision); err != nil {
		return LifecycleJobResult{}, err
	}
	if revision != command.ObservedLedgerRevision {
		return LifecycleJobResult{}, ErrCodingLedgerRevision
	}
	if _, err := codingRecoveryStepTx(ctx, tx, job); err != nil {
		return LifecycleJobResult{}, err
	}
	job, err = resumeFrozenCodingTx(ctx, tx, job, command.Feedback)
	if err != nil {
		return LifecycleJobResult{}, err
	}
	err = insertLifecycleOperationTx(ctx, tx, descriptor, lifecycleOperationRecord{ID: command.OperationID, JobID: job.ID, ObservedGeneration: job.CurrentGeneration, ResultGeneration: job.CurrentGeneration, Kind: LifecycleResumeJob, ResultJobStatus: model.JobStatusRunning, ResultJob: job})
	return LifecycleJobResult{Job: job, Applied: true}, err
}

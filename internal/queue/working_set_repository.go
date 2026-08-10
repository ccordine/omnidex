package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) CreateCurrentWorkingSet(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	budget workingset.Budget,
) (workingset.Snapshot, error) {
	if err := validateWorkingSetRequest(r, ctx, authority.JobID, authority.Generation); err != nil {
		return workingset.Snapshot{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return workingset.Snapshot{}, fmt.Errorf("begin working-set creation: %w", err)
	}
	defer tx.Rollback(ctx)
	jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, authority)
	if err != nil {
		return workingset.Snapshot{}, err
	}
	if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
		return workingset.Snapshot{}, staleStepAttemptError(authority, "working-set creator is not running", nil)
	}
	header, err := loadTaskLedgerHeaderTx(ctx, tx, authority.JobID, false)
	if err != nil {
		return workingset.Snapshot{}, err
	}
	if err := requireCurrentWorkingSetAuthority(header, authority.Generation); err != nil {
		return workingset.Snapshot{}, err
	}
	owner := workingset.Owner{
		LedgerID: header.ID, JobID: authority.JobID, Generation: authority.Generation,
	}
	set, err := workingset.New(owner, budget)
	if err != nil {
		return workingset.Snapshot{}, err
	}
	snapshot := set.Snapshot()
	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM working_sets WHERE job_id=$1 AND generation=$2
		)
	`, authority.JobID, authority.Generation).Scan(&exists); err != nil {
		return workingset.Snapshot{}, fmt.Errorf("check working set for job %d: %w", authority.JobID, err)
	}
	if exists {
		return workingset.Snapshot{}, fmt.Errorf(
			"%w: job %d generation %d", ErrWorkingSetExists, authority.JobID, authority.Generation,
		)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO working_sets (
			id, ledger_id, job_id, generation, scope_kind, scope_id,
			max_items, max_bytes, max_pinned_items, max_pinned_bytes,
			status, version, clock, closed_tick, close_reason
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
	`, snapshot.ID, snapshot.Owner.LedgerID, authority.JobID, authority.Generation,
		snapshot.Scope.Kind, snapshot.Scope.ID, snapshot.Budget.MaxItems, snapshot.Budget.MaxBytes,
		snapshot.Budget.MaxPinnedItems, snapshot.Budget.MaxPinnedBytes,
		snapshot.Status, int64(snapshot.Version), int64(snapshot.Clock), int64(snapshot.ClosedTick), snapshot.CloseReason,
	); err != nil {
		return workingset.Snapshot{}, fmt.Errorf("create working set %q: %w", snapshot.ID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return workingset.Snapshot{}, fmt.Errorf("commit working-set creation: %w", err)
	}
	return snapshot, nil
}

func (r *Repository) CurrentWorkingSet(ctx context.Context, jobID int64) (workingset.Snapshot, error) {
	if err := validateWorkingSetRead(r, ctx, jobID); err != nil {
		return workingset.Snapshot{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return workingset.Snapshot{}, fmt.Errorf("begin current working-set read: %w", err)
	}
	defer tx.Rollback(ctx)
	header, err := loadTaskLedgerHeaderTx(ctx, tx, jobID, false)
	if err != nil {
		return workingset.Snapshot{}, err
	}
	snapshot, err := loadWorkingSetSnapshotTx(ctx, tx, header, header.Generation, false)
	if err != nil {
		return workingset.Snapshot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return workingset.Snapshot{}, fmt.Errorf("commit current working-set read: %w", err)
	}
	return snapshot, nil
}

func (r *Repository) WorkingSetForGeneration(
	ctx context.Context,
	jobID, generation int64,
) (workingset.Snapshot, error) {
	if err := validateWorkingSetRequest(r, ctx, jobID, generation); err != nil {
		return workingset.Snapshot{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return workingset.Snapshot{}, fmt.Errorf("begin historical working-set read: %w", err)
	}
	defer tx.Rollback(ctx)
	header, err := loadTaskLedgerHeaderTx(ctx, tx, jobID, false)
	if err != nil {
		return workingset.Snapshot{}, err
	}
	if generation > header.Generation {
		return workingset.Snapshot{}, fmt.Errorf(
			"%w: job %d has no generation %d", ErrInvalidJobGeneration, jobID, generation,
		)
	}
	snapshot, err := loadWorkingSetSnapshotTx(ctx, tx, header, generation, false)
	if err != nil {
		return workingset.Snapshot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return workingset.Snapshot{}, fmt.Errorf("commit historical working-set read: %w", err)
	}
	return snapshot, nil
}

func validateWorkingSetRead(r *Repository, ctx context.Context, jobID int64) error {
	if ctx == nil {
		return fmt.Errorf("working-set context is required")
	}
	if jobID <= 0 {
		return fmt.Errorf("working-set job ID must be positive")
	}
	if r == nil || r.pool == nil {
		return fmt.Errorf("postgres repository is not configured")
	}
	return nil
}

func validateWorkingSetRequest(r *Repository, ctx context.Context, jobID, generation int64) error {
	if ctx == nil {
		return fmt.Errorf("working-set context is required")
	}
	if jobID <= 0 {
		return fmt.Errorf("working-set job ID must be positive")
	}
	if generation <= 0 {
		return fmt.Errorf("working-set generation must be positive")
	}
	if r == nil || r.pool == nil {
		return fmt.Errorf("postgres repository is not configured")
	}
	return nil
}

func requireCurrentWorkingSetAuthority(header taskLedgerHeader, generation int64) error {
	if header.Generation != generation {
		return fmt.Errorf(
			"%w: working set observed job %d generation %d, current generation is %d",
			ErrStaleJobGeneration, header.Owner.JobID, generation, header.Generation,
		)
	}
	if terminalJobStatus(header.JobStatus) {
		return fmt.Errorf("working set cannot mutate terminal job %d status %q", header.Owner.JobID, header.JobStatus)
	}
	if header.JobStatus != model.JobStatusPending && header.JobStatus != model.JobStatusRunning &&
		header.JobStatus != model.JobStatusWaiting {
		return fmt.Errorf("working set found unregistered job status %q", header.JobStatus)
	}
	if header.Status != taskstate.LedgerActive {
		return fmt.Errorf(
			"%w: nonterminal job %d has task ledger status %q",
			taskstate.ErrInvalidState, header.Owner.JobID, header.Status,
		)
	}
	return nil
}

func workingSetNotFound(jobID, generation int64) error {
	return fmt.Errorf("%w: job %d generation %d", ErrWorkingSetNotFound, jobID, generation)
}

func isWorkingSetNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

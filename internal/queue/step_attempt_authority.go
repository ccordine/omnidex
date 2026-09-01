package queue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func validateStepAttemptAuthority(authority model.StepAttemptAuthority) error {
	if authority.JobID <= 0 || authority.Generation <= 0 || authority.StepID <= 0 || authority.Attempt <= 0 {
		return fmt.Errorf("%w: exact positive job, generation, step, and attempt identities are required", ErrStaleStepAttempt)
	}
	worker := authority.WorkerID
	if worker == "" || worker != strings.TrimSpace(worker) || len(worker) > 256 ||
		!utf8.ValidString(worker) || strings.ContainsRune(worker, '\x00') {
		return fmt.Errorf("%w: worker identity must be exact PostgreSQL-compatible text of at most 256 bytes", ErrStaleStepAttempt)
	}
	return nil
}

func requireActiveStepAttemptTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
) (jobStatus string, stepStatus string, expiresAt time.Time, err error) {
	locked, err := lockStepAttemptAuthorityTx(ctx, tx, authority)
	if err != nil {
		return "", "", time.Time{}, err
	}
	if err := requireLockedStepAttemptActiveTx(ctx, tx, authority, locked); err != nil {
		return "", "", time.Time{}, err
	}
	return locked.JobStatus, locked.StepStatus, locked.ExpiresAt, nil
}

func (r *Repository) RequireActiveStepAttempt(
	ctx context.Context,
	authority model.StepAttemptAuthority,
) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("validate active step attempt: PostgreSQL repository is unavailable")
	}
	if ctx == nil {
		return fmt.Errorf("validate active step attempt: context is required")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin active step attempt validation: %w", err)
	}
	defer tx.Rollback(ctx)
	jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, authority)
	if err != nil {
		return err
	}
	if stepStatus != model.StepStatusRunning || !jobAcceptsStepTerminal(jobStatus) {
		return staleStepAttemptError(
			authority,
			fmt.Sprintf("workspace mutation writer job status %q step status %q", jobStatus, stepStatus),
			nil,
		)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit active step attempt validation: %w", err)
	}
	return nil
}

func requireLockedStepAttemptActiveTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	locked lockedStepAttempt,
) error {
	if locked.AttemptStatus != model.StepAttemptActive {
		return staleStepAttemptError(authority, "attempt is not active", nil)
	}
	var active bool
	if err := tx.QueryRow(ctx, `SELECT $1::timestamptz > clock_timestamp()`, locked.ExpiresAt).Scan(&active); err != nil {
		return err
	}
	if !active {
		return staleStepAttemptError(authority, "attempt lease expired", nil)
	}
	return nil
}

type lockedStepAttempt struct {
	JobStatus     string
	StepStatus    string
	AttemptStatus model.StepAttemptStatus
	ExpiresAt     time.Time
}

func lockStepAttemptAuthorityTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
) (lockedStepAttempt, error) {
	if err := validateStepAttemptAuthority(authority); err != nil {
		return lockedStepAttempt{}, err
	}
	var locked lockedStepAttempt
	var currentGeneration int64
	if err := tx.QueryRow(ctx, `
		SELECT status,current_generation FROM jobs WHERE id=$1 FOR UPDATE
	`, authority.JobID).Scan(&locked.JobStatus, &currentGeneration); err != nil {
		return lockedStepAttempt{}, staleStepAttemptError(authority, "job authority is unavailable", err)
	}
	if currentGeneration != authority.Generation {
		return lockedStepAttempt{}, staleStepAttemptError(authority, "job generation changed", nil)
	}

	var stepGeneration, currentAttempt int64
	var supersededAt *int64
	var workerID *string
	if err := tx.QueryRow(ctx, `
		SELECT status,generation,superseded_at_generation,current_attempt,worker_id
		FROM job_steps WHERE job_id=$1 AND id=$2 FOR UPDATE
	`, authority.JobID, authority.StepID).Scan(
		&locked.StepStatus, &stepGeneration, &supersededAt, &currentAttempt, &workerID,
	); err != nil {
		return lockedStepAttempt{}, staleStepAttemptError(authority, "step authority is unavailable", err)
	}
	if stepGeneration != authority.Generation || supersededAt != nil ||
		currentAttempt != authority.Attempt || workerID == nil || *workerID != authority.WorkerID {
		return lockedStepAttempt{}, staleStepAttemptError(authority, "step authority changed", nil)
	}

	var attemptWorker string
	if err := tx.QueryRow(ctx, `
		SELECT status,worker_id,expires_at
		FROM job_step_attempts
		WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4
		FOR UPDATE
	`, authority.JobID, authority.Generation, authority.StepID, authority.Attempt).Scan(
		&locked.AttemptStatus, &attemptWorker, &locked.ExpiresAt,
	); err != nil {
		return lockedStepAttempt{}, staleStepAttemptError(authority, "attempt authority is unavailable", err)
	}
	if attemptWorker != authority.WorkerID {
		return lockedStepAttempt{}, staleStepAttemptError(authority, "attempt worker changed", nil)
	}
	return locked, nil
}

func staleStepAttemptError(authority model.StepAttemptAuthority, reason string, cause error) error {
	message := fmt.Errorf(
		"%w: job %d generation %d step %d attempt %d worker %q: %s",
		ErrStaleStepAttempt, authority.JobID, authority.Generation, authority.StepID,
		authority.Attempt, authority.WorkerID, reason,
	)
	if cause == nil || errors.Is(cause, pgx.ErrNoRows) {
		return message
	}
	return fmt.Errorf("validate step attempt database authority: %w", cause)
}

func terminalizeStepAttemptTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	status model.StepAttemptStatus,
) error {
	if status == model.StepAttemptActive {
		return fmt.Errorf("terminalize step attempt: active is not a terminal status")
	}
	result, err := tx.Exec(ctx, `
		UPDATE job_step_attempts
		SET status=$5,finished_at=clock_timestamp()
		WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4
		  AND worker_id=$6 AND status=$7 AND expires_at>clock_timestamp()
	`, authority.JobID, authority.Generation, authority.StepID, authority.Attempt,
		status, authority.WorkerID, model.StepAttemptActive)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return staleStepAttemptError(authority, "attempt lost terminal transition authority", nil)
	}
	return nil
}

func (r *Repository) RenewStepAttempt(
	ctx context.Context,
	authority model.StepAttemptAuthority,
) (time.Time, error) {
	if r == nil || r.pool == nil {
		return time.Time{}, fmt.Errorf("renew step attempt: PostgreSQL repository is unavailable")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return time.Time{}, err
	}
	defer tx.Rollback(ctx)
	if _, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, authority); err != nil {
		return time.Time{}, err
	} else if stepStatus != model.StepStatusRunning {
		return time.Time{}, staleStepAttemptError(authority, "step is not running", nil)
	}
	leaseMeasurementStarted := time.Now()
	var leaseValidForNanoseconds int64
	err = tx.QueryRow(ctx, `
		UPDATE job_step_attempts
		SET renewed_at=clock_timestamp()
		WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4
		  AND status=$5 AND expires_at>clock_timestamp()
		RETURNING GREATEST(
			FLOOR(EXTRACT(EPOCH FROM (expires_at-clock_timestamp()))*1000000000)::bigint,
			0::bigint
		)
	`, authority.JobID, authority.Generation, authority.StepID, authority.Attempt,
		model.StepAttemptActive).Scan(&leaseValidForNanoseconds)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, staleStepAttemptError(authority, "attempt expired before renewal", nil)
	}
	if err != nil {
		return time.Time{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return time.Time{}, err
	}
	leaseDeadline := leaseMeasurementStarted.Add(time.Duration(leaseValidForNanoseconds))
	if time.Until(leaseDeadline) <= 0 {
		return time.Time{}, staleStepAttemptError(authority, "attempt lease expired before renewal delivery", nil)
	}
	return leaseDeadline, nil
}

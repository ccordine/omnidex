package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func lockCognitionReplacementStepTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
) error {
	if err := validateStepAttemptAuthority(authority); err != nil {
		return err
	}
	var jobStatus string
	var generation int64
	if err := tx.QueryRow(ctx, `SELECT status,current_generation FROM jobs WHERE id=$1 FOR UPDATE`,
		authority.JobID).Scan(&jobStatus, &generation); err != nil {
		return staleStepAttemptError(authority, "job authority is unavailable", err)
	}
	var stepStatus, worker string
	var stepGeneration, currentAttempt int64
	var superseded *int64
	if err := tx.QueryRow(ctx, `
		SELECT status,generation,superseded_at_generation,current_attempt,COALESCE(worker_id,'')
		FROM job_steps WHERE job_id=$1 AND id=$2 FOR UPDATE
	`, authority.JobID, authority.StepID).Scan(
		&stepStatus, &stepGeneration, &superseded, &currentAttempt, &worker,
	); err != nil {
		return staleStepAttemptError(authority, "step authority is unavailable", err)
	}
	if jobStatus != model.JobStatusRunning || generation != authority.Generation ||
		stepStatus != model.StepStatusRunning || stepGeneration != authority.Generation || superseded != nil ||
		currentAttempt != authority.Attempt || worker != authority.WorkerID {
		return staleStepAttemptError(authority, "replacement step authority changed", nil)
	}
	return nil
}

func lockCurrentCognitionReplacementAttemptTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
) error {
	var status model.StepAttemptStatus
	var worker string
	var active bool
	err := tx.QueryRow(ctx, `
		SELECT status,worker_id,expires_at>clock_timestamp()
		FROM job_step_attempts
		WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4
		FOR UPDATE
	`, authority.JobID, authority.Generation, authority.StepID, authority.Attempt).Scan(&status, &worker, &active)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && worker != authority.WorkerID) {
		return staleStepAttemptError(authority, "replacement attempt changed", nil)
	}
	if err != nil {
		return err
	}
	if status != model.StepAttemptActive || !active {
		return staleStepAttemptError(authority, "replacement attempt is not active", nil)
	}
	return nil
}

func loadPendingCognitionAbandonmentTx(
	ctx context.Context,
	tx pgx.Tx,
	binding cognitionruntime.Binding,
	lock bool,
) (cognitionruntime.PolicyCallAbandonment, bool, error) {
	query := `
		SELECT abandonments.descriptor_json
		FROM cognition_policy_call_abandonments abandonments
		JOIN cognition_runtime_snapshots source ON source.snapshot_sha256=abandonments.source_snapshot_sha256
		WHERE abandonments.episode_id=$1 AND abandonments.job_id=$2 AND abandonments.generation=$3
		  AND abandonments.step_id=$4 AND abandonments.recovery_attempt=$5
		  AND abandonments.recovery_worker_id=$6
		  AND NOT EXISTS (
		      SELECT 1 FROM cognition_policy_calls later
		      JOIN cognition_runtime_snapshots snapshots ON snapshots.snapshot_sha256=later.snapshot_sha256
		      WHERE later.episode_id=abandonments.episode_id AND snapshots.call_ordinal>abandonments.call_ordinal
		  )
		ORDER BY abandonments.created_at DESC,abandonments.abandonment_id LIMIT 2`
	if lock {
		query += ` FOR UPDATE OF abandonments`
	}
	rows, err := tx.Query(ctx, query, binding.Episode.ID, binding.Attempt.JobID,
		binding.Attempt.Generation, binding.Attempt.StepID, int64(binding.Attempt.Attempt), binding.Attempt.WorkerID)
	if err != nil {
		return cognitionruntime.PolicyCallAbandonment{}, false, err
	}
	defer rows.Close()
	var values []cognitionruntime.PolicyCallAbandonment
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return cognitionruntime.PolicyCallAbandonment{}, false, err
		}
		value, err := decodeCognitionPolicyAbandonment(raw)
		if err != nil {
			return cognitionruntime.PolicyCallAbandonment{}, false, fmt.Errorf("decode cognition abandonment: %w", err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return cognitionruntime.PolicyCallAbandonment{}, false, err
	}
	if len(values) > 1 {
		return cognitionruntime.PolicyCallAbandonment{}, false, fmt.Errorf("%w: multiple pending abandonment receipts", ErrCognitionConflict)
	}
	if len(values) == 0 {
		return cognitionruntime.PolicyCallAbandonment{}, false, nil
	}
	if err := values[0].ValidateFor(binding); err != nil {
		return cognitionruntime.PolicyCallAbandonment{}, false, err
	}
	return values[0], true, nil
}

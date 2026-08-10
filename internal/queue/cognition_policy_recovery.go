package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ReplayTerminalCognitionPolicyOutcome(
	ctx context.Context,
	binding cognitionruntime.Binding,
) (bool, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return false, fmt.Errorf("terminal cognition policy recovery requires PostgreSQL and context")
	}
	authority, err := cognitionRuntimeAuthority(binding)
	if err != nil {
		return false, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	if _, status, _, err := requireActiveStepAttemptTx(ctx, tx, authority); err != nil {
		return false, err
	} else if status != model.StepStatusRunning {
		return false, staleStepAttemptError(authority, "policy recovery actor is not running", nil)
	}
	callID, found, err := findTerminalCognitionPolicyCallTx(ctx, tx, authority, binding.Episode.ID)
	if err != nil || !found {
		return false, err
	}
	record, found, err := loadCognitionPolicyCallTx(ctx, tx, callID, true)
	if err != nil || !found || record.Result == nil {
		return false, fmt.Errorf("%w: terminal policy call is unavailable: %v", ErrCognitionConflict, err)
	}
	if err := validateRecoverableCognitionPolicyCallTx(ctx, tx, authority, binding, record); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, terminalCognitionPolicyError(*record.Result)
}

func findTerminalCognitionPolicyCallTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	episodeID cognition.EpisodeID,
) (string, bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT calls.call_id
		FROM cognition_policy_calls calls
		LEFT JOIN cognition_reconciliations reconciliations ON reconciliations.policy_call_id=calls.call_id
		LEFT JOIN cognition_actions actions ON actions.policy_call_id=calls.call_id
		WHERE calls.episode_id=$1 AND calls.job_id=$2 AND calls.generation=$3 AND calls.step_id=$4
		  AND (calls.step_attempt<$5 OR (calls.step_attempt=$5 AND calls.worker_id=$6))
		  AND calls.status IN ('rejected','failed')
		  AND reconciliations.reconciliation_id IS NULL AND actions.action_id IS NULL
		ORDER BY calls.step_attempt DESC,calls.created_at DESC,calls.call_id
		LIMIT 2
	`, episodeID, authority.JobID, authority.Generation, authority.StepID,
		authority.Attempt, authority.WorkerID)
	if err != nil {
		return "", false, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", false, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return "", false, err
	}
	if len(ids) > 1 {
		return "", false, fmt.Errorf("%w: multiple terminal policy outcomes are recoverable", ErrCognitionConflict)
	}
	if len(ids) == 0 {
		return "", false, nil
	}
	return ids[0], true, nil
}

func validateRecoverableCognitionPolicyCallTx(
	ctx context.Context,
	tx pgx.Tx,
	current model.StepAttemptAuthority,
	binding cognitionruntime.Binding,
	record cognitionPolicyCallRecord,
) error {
	source, err := cognitionPolicyCallAuthority(record.Attempt)
	if err != nil {
		return err
	}
	if record.Attempt.ExpectedRevision.EpisodeID != binding.Episode.ID ||
		!sameQueueStepAttempt(record.Attempt.Actor, binding.Attempt) ||
		record.Attempt.Actor.Attempt > binding.Attempt.Attempt {
		return fmt.Errorf("%w: terminal policy outcome belongs to another authority", ErrCognitionConflict)
	}
	if source != current {
		status, err := loadSourceCognitionAttemptStatusTx(ctx, tx, source)
		if err != nil {
			return err
		}
		if status != model.StepAttemptExpired && status != model.StepAttemptSuperseded {
			return fmt.Errorf("%w: terminal policy source attempt is not replaced", ErrCognitionConflict)
		}
	}
	return validateCognitionPolicyCallEpisodeTx(ctx, tx, record)
}

func terminalCognitionPolicyError(result cognitionpolicy.CallResult) error {
	err := cognitionpolicy.CallResultError(result)
	if errors.Is(err, cognitionpolicy.ErrInvalidEvidence) &&
		result.FailureCode != cognitionpolicy.CallFailureProviderRequest &&
		result.FailureCode != cognitionpolicy.CallFailurePolicyAuthority &&
		result.FailureCode != cognitionpolicy.CallFailureProviderEvidence {
		return fmt.Errorf("%w: terminal policy failure mapping: %v", ErrCognitionConflict, err)
	}
	return fmt.Errorf("%w: durable policy call %s", err, result.CallID)
}

func loadSourceCognitionAttemptStatusTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
) (model.StepAttemptStatus, error) {
	var status model.StepAttemptStatus
	var worker string
	err := tx.QueryRow(ctx, `
		SELECT status,worker_id FROM job_step_attempts
		WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4
		FOR UPDATE
	`, authority.JobID, authority.Generation, authority.StepID, authority.Attempt).Scan(&status, &worker)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && worker != authority.WorkerID) {
		return "", staleStepAttemptError(authority, "policy source attempt changed", nil)
	}
	if err != nil {
		return "", err
	}
	return status, nil
}

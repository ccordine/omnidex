package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) TransitionGeneratedWorkloadDeployment(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	command GeneratedWorkloadDeploymentCommand,
	transition GeneratedWorkloadDeploymentTransition,
) (GeneratedWorkloadDeploymentRecord, error) {
	if ctx == nil {
		return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf("transition generated deployment requires a context")
	}
	if err := ctx.Err(); err != nil {
		return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf("transition generated deployment: %w", err)
	}
	if r == nil || r.pool == nil {
		return GeneratedWorkloadDeploymentRecord{}, ErrRepositoryNotConfigured
	}
	if err := validateGeneratedDeploymentExecutionAuthority(authority, command); err != nil {
		return GeneratedWorkloadDeploymentRecord{}, err
	}
	if err := validateGeneratedWorkloadDeploymentTransition(transition); err != nil {
		return GeneratedWorkloadDeploymentRecord{}, err
	}
	if transition.State == GeneratedWorkloadDeploymentRolledBack {
		return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf(
			"%w: rolled-back state requires one atomic clean rollback observation",
			ErrGeneratedWorkloadDeploymentState,
		)
	}
	identity, err := generatedWorkloadDeploymentOperation(command)
	if err != nil {
		return GeneratedWorkloadDeploymentRecord{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf("begin generated deployment transition: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockGeneratedDeploymentAuthorityTx(ctx, tx, authority, command); err != nil {
		return GeneratedWorkloadDeploymentRecord{}, err
	}
	record, err := lockGeneratedDeploymentTx(ctx, tx, identity.OperationID)
	if err != nil {
		return GeneratedWorkloadDeploymentRecord{}, err
	}
	if err := requireGeneratedDeploymentIdentity(record, identity); err != nil {
		return GeneratedWorkloadDeploymentRecord{}, err
	}
	if record.State == transition.State {
		if transition.State == GeneratedWorkloadDeploymentApplying &&
			(record.Current.StepAttempt != authority.Attempt || record.Current.WorkerID != authority.WorkerID) {
			return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf(
				"%w: applying replay requires atomic project-head takeover first",
				ErrGeneratedWorkloadDeploymentState,
			)
		}
		if transition.State != GeneratedWorkloadDeploymentApplying &&
			(record.TerminalCode != transition.Code || record.DetailSHA256 != transition.DetailSHA256) {
			return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf(
				"%w: replay changed the %s transition detail", ErrGeneratedWorkloadDeploymentConflict, transition.State,
			)
		}
		if transition.State == GeneratedWorkloadDeploymentFailed {
			if err := releaseGeneratedWorkloadProjectDeploymentCandidateTx(
				ctx, tx, command, identity.OperationID, record.Current, false,
			); err != nil {
				return GeneratedWorkloadDeploymentRecord{}, err
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf("commit generated deployment transition replay: %w", err)
		}
		return record, nil
	}
	if !generatedDeploymentTransitionAllowed(record.State, transition.State) {
		return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf(
			"%w: transition from %s to %s is unavailable",
			ErrGeneratedWorkloadDeploymentState, record.State, transition.State,
		)
	}
	if transition.State == GeneratedWorkloadDeploymentApplying {
		var cleanupStarted bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
			 SELECT 1 FROM generated_workload_deployment_rollback_attempts WHERE operation_id=$1
			 UNION ALL
			 SELECT 1 FROM generated_workload_deployment_rollback_observations
			 WHERE operation_id=$1 AND basis='pre_attempt'
			 UNION ALL
			 SELECT 1 FROM generated_workload_deployment_executions
			 WHERE operation_id=$1
			)
		`, identity.OperationID).Scan(&cleanupStarted); err != nil {
			return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf("inspect deployment cleanup phase: %w", err)
		}
		if cleanupStarted {
			return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf(
				"%w: deployment has entered one-way cleanup reconciliation",
				ErrGeneratedWorkloadDeploymentState,
			)
		}
	}
	if transition.State == GeneratedWorkloadDeploymentFailed &&
		(record.State == GeneratedWorkloadDeploymentApplying ||
			record.State == GeneratedWorkloadDeploymentIndeterminate) {
		var resourceSideEffectPossible bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
			 SELECT 1 FROM generated_workload_deployment_executions
			 WHERE operation_id=$1
			 UNION ALL
			 SELECT 1 FROM generated_workload_deployment_rollback_attempts WHERE operation_id=$1
			 UNION ALL
			 SELECT 1 FROM generated_workload_deployment_rollback_observations
			 WHERE operation_id=$1 AND basis='pre_attempt'
			)
		`, identity.OperationID).Scan(
			&resourceSideEffectPossible,
		); err != nil {
			return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf("inspect deployment side-effect rail: %w", err)
		}
		if resourceSideEffectPossible {
			return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf(
				"%w: side-effect-possible deployment failure requires observe-first cleanup",
				ErrGeneratedWorkloadDeploymentState,
			)
		}
	}
	var terminalCode, detailSHA any
	if transition.State != GeneratedWorkloadDeploymentApplying {
		terminalCode, detailSHA = transition.Code, transition.DetailSHA256
	}
	_, err = tx.Exec(ctx, `
		UPDATE generated_workload_deployments
		SET status=$2,
		    attempt_count=attempt_count+CASE WHEN $2='applying' THEN 1 ELSE 0 END,
		    terminal_code=$3,terminal_detail_sha256=$4,
		    current_step_attempt=$5,current_worker_id=$6,
		    applying_at=CASE WHEN $2='applying' THEN COALESCE(applying_at,clock_timestamp()) ELSE applying_at END,
		    terminal_at=CASE WHEN $2 IN ('failed','rolled_back') THEN clock_timestamp() ELSE NULL END,
		    updated_at=clock_timestamp()
		WHERE id=$1
	`, identity.OperationID, transition.State, terminalCode, detailSHA,
		authority.Attempt, authority.WorkerID)
	if err != nil {
		return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf("update generated deployment transition: %w", err)
	}
	record, err = lockGeneratedDeploymentTx(ctx, tx, identity.OperationID)
	if err != nil {
		return GeneratedWorkloadDeploymentRecord{}, err
	}
	if transition.State == GeneratedWorkloadDeploymentFailed {
		requireCandidate := record.State == GeneratedWorkloadDeploymentFailed &&
			(record.AttemptCount > 0 || record.Creator != record.Current)
		if err := releaseGeneratedWorkloadProjectDeploymentCandidateTx(
			ctx, tx, command, identity.OperationID, record.Current, requireCandidate,
		); err != nil {
			return GeneratedWorkloadDeploymentRecord{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf("commit generated deployment transition: %w", err)
	}
	return record, nil
}

func generatedDeploymentTransitionAllowed(
	from, to GeneratedWorkloadDeploymentState,
) bool {
	switch from {
	case GeneratedWorkloadDeploymentPrepared:
		return to == GeneratedWorkloadDeploymentApplying || to == GeneratedWorkloadDeploymentFailed
	case GeneratedWorkloadDeploymentApplying:
		return to == GeneratedWorkloadDeploymentFailed ||
			to == GeneratedWorkloadDeploymentIndeterminate
	case GeneratedWorkloadDeploymentIndeterminate:
		return to == GeneratedWorkloadDeploymentApplying ||
			to == GeneratedWorkloadDeploymentFailed
	default:
		return false
	}
}

package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func canonicalGeneratedDeploymentRollbackObservation(
	plan GeneratedWorkloadDeploymentRollbackPlan,
	observation GeneratedWorkloadDeploymentRollbackObservation,
) (string, string, error) {
	providedSHA := observation.SHA256
	bound, encoded, err := BindGeneratedWorkloadDeploymentRollbackObservation(plan, observation)
	if err != nil {
		return "", "", err
	}
	if providedSHA != bound.SHA256 {
		return "", "", fmt.Errorf("rollback observation SHA-256 differs from exact JSON bytes")
	}
	outcome := GeneratedWorkloadDeploymentRollbackResidual
	if len(bound.ContainerIDs) == 0 && len(bound.NetworkIDs) == 0 && len(bound.VolumeNames) == 0 {
		outcome = GeneratedWorkloadDeploymentRollbackClean
	}
	return encoded, outcome, nil
}

func BindGeneratedWorkloadDeploymentRollbackObservation(
	plan GeneratedWorkloadDeploymentRollbackPlan,
	observation GeneratedWorkloadDeploymentRollbackObservation,
) (GeneratedWorkloadDeploymentRollbackObservation, string, error) {
	if observation.Schema != GeneratedWorkloadDeploymentRollbackObservationV1 ||
		observation.ComposeProject != plan.ComposeProject ||
		observation.PostconditionSHA256 != plan.PostconditionSHA256 ||
		observation.ContainerIDs == nil || observation.NetworkIDs == nil ||
		observation.VolumeNames == nil {
		return GeneratedWorkloadDeploymentRollbackObservation{}, "",
			fmt.Errorf("rollback observation authority is incomplete")
	}
	if err := validateGeneratedDeploymentRollbackResources(
		"container", observation.ContainerIDs, repositoryMutationHexDigest,
	); err != nil {
		return GeneratedWorkloadDeploymentRollbackObservation{}, "", err
	}
	if err := validateGeneratedDeploymentRollbackResources(
		"network", observation.NetworkIDs, repositoryMutationHexDigest,
	); err != nil {
		return GeneratedWorkloadDeploymentRollbackObservation{}, "", err
	}
	if err := validateGeneratedDeploymentRollbackResources(
		"volume", observation.VolumeNames, generatedDeploymentRollbackVolume.MatchString,
	); err != nil {
		return GeneratedWorkloadDeploymentRollbackObservation{}, "", err
	}
	observation.SHA256 = ""
	encoded, err := canonicalGeneratedDeploymentJSON(observation)
	if err != nil || len(encoded) > 32768 {
		return GeneratedWorkloadDeploymentRollbackObservation{}, "",
			fmt.Errorf("rollback observation exceeds canonical bound: %w", err)
	}
	observation.SHA256 = generatedDeploymentSHA(encoded)
	return observation, encoded, nil
}

func validateGeneratedDeploymentRollbackResources(
	name string,
	resources []string,
	valid func(string) bool,
) error {
	if len(resources) > 1024 {
		return fmt.Errorf("rollback %s observation exceeds resource bound", name)
	}
	previous := ""
	for index, resource := range resources {
		if !valid(resource) || index > 0 && resource <= previous {
			return fmt.Errorf("rollback %s resources must be canonical, sorted, and unique", name)
		}
		previous = resource
	}
	return nil
}

func loadGeneratedDeploymentRollbackObservationTx(
	ctx context.Context,
	querier generatedDeploymentExecutionQuerier,
	operationID string,
	rollbackStepAttempt int64,
	lock bool,
) (GeneratedWorkloadDeploymentRollbackObservationRecord, bool, error) {
	query := `
		SELECT operation_id,rollback_step_attempt,observer_step_attempt,
		       observer_worker_id,basis,outcome,observation_json,observation_sha256,
		       evidence_id,observed_at
		FROM generated_workload_deployment_rollback_observations
		WHERE operation_id=$1 AND rollback_step_attempt=$2`
	if lock {
		query += ` FOR UPDATE`
	}
	var record GeneratedWorkloadDeploymentRollbackObservationRecord
	var observationJSON string
	err := querier.QueryRow(ctx, query, operationID, rollbackStepAttempt).Scan(
		&record.OperationID, &record.RollbackStepAttempt,
		&record.ObserverStepAttempt, &record.ObserverWorkerID, &record.Basis, &record.Outcome,
		&observationJSON, &record.Observation.SHA256, &record.EvidenceID,
		&record.ObservedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return GeneratedWorkloadDeploymentRollbackObservationRecord{}, false, nil
	}
	if err != nil {
		return GeneratedWorkloadDeploymentRollbackObservationRecord{}, false,
			fmt.Errorf("load rollback observation: %w", err)
	}
	if err := decodeExactGeneratedDeploymentJSON(
		observationJSON, &record.Observation,
	); err != nil {
		return GeneratedWorkloadDeploymentRollbackObservationRecord{}, false,
			fmt.Errorf("decode durable rollback observation: %w", err)
	}
	canonical, err := canonicalGeneratedDeploymentJSON(record.Observation)
	if err != nil || canonical != observationJSON ||
		generatedDeploymentSHA(observationJSON) != record.Observation.SHA256 ||
		(record.Basis != GeneratedWorkloadDeploymentRollbackObservationCommandAttempt &&
			record.Basis != GeneratedWorkloadDeploymentRollbackObservationPreAttempt) ||
		(record.Outcome != GeneratedWorkloadDeploymentRollbackClean &&
			record.Outcome != GeneratedWorkloadDeploymentRollbackResidual) {
		return GeneratedWorkloadDeploymentRollbackObservationRecord{}, false,
			fmt.Errorf("durable rollback observation is not canonical")
	}
	return record, true, nil
}

func (r *Repository) finishGeneratedDeploymentRollbackObservationTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	command GeneratedWorkloadDeploymentCommand,
	operationID string,
	deployment GeneratedWorkloadDeploymentRecord,
	observation GeneratedWorkloadDeploymentRollbackObservationRecord,
	terminal GeneratedWorkloadDeploymentTransition,
) (GeneratedWorkloadDeploymentRecord, GeneratedWorkloadDeploymentRollbackObservationRecord, error) {
	quiescent, err := generatedDeploymentForwardQuiescentTx(ctx, tx, operationID)
	if err != nil {
		return GeneratedWorkloadDeploymentRecord{}, observation, err
	}
	if !quiescent {
		if deployment.State == GeneratedWorkloadDeploymentApplying {
			if _, err := tx.Exec(ctx, `
				UPDATE generated_workload_deployments
				SET status='indeterminate',terminal_code='external_quiescence_unproven',
				    terminal_detail_sha256=$2,terminal_at=NULL,
				    updated_at=GREATEST(clock_timestamp(),updated_at+INTERVAL '1 microsecond')
				WHERE id=$1
			`, operationID, terminal.DetailSHA256); err != nil {
				return GeneratedWorkloadDeploymentRecord{}, observation,
					fmt.Errorf("fence unacknowledged deployment execution: %w", err)
			}
		} else if deployment.State == GeneratedWorkloadDeploymentIndeterminate {
			if deployment.TerminalCode != "external_quiescence_unproven" ||
				deployment.DetailSHA256 != terminal.DetailSHA256 {
				return GeneratedWorkloadDeploymentRecord{}, observation, fmt.Errorf(
					"%w: indeterminate deployment quiescence detail differs",
					ErrGeneratedWorkloadDeploymentConflict,
				)
			}
		} else {
			return GeneratedWorkloadDeploymentRecord{}, observation, fmt.Errorf(
				"%w: unacknowledged execution cannot close from %s",
				ErrGeneratedWorkloadDeploymentState, deployment.State,
			)
		}
		updated, err := lockGeneratedDeploymentTx(ctx, tx, operationID)
		if err != nil {
			return GeneratedWorkloadDeploymentRecord{}, observation, err
		}
		if err := tx.Commit(ctx); err != nil {
			return GeneratedWorkloadDeploymentRecord{}, observation,
				fmt.Errorf("commit unacknowledged deployment observation: %w", err)
		}
		return updated, observation, nil
	}
	if observation.Outcome == GeneratedWorkloadDeploymentRollbackClean {
		if deployment.State == GeneratedWorkloadDeploymentRolledBack {
			if deployment.TerminalCode != terminal.Code ||
				deployment.DetailSHA256 != terminal.DetailSHA256 {
				return GeneratedWorkloadDeploymentRecord{}, observation,
					fmt.Errorf("%w: rolled-back replay detail differs", ErrGeneratedWorkloadDeploymentConflict)
			}
			if err := releaseGeneratedWorkloadProjectDeploymentCandidateTx(
				ctx, tx, command, operationID, deployment.Current, false,
			); err != nil {
				return GeneratedWorkloadDeploymentRecord{}, observation, err
			}
		} else {
			if deployment.State != GeneratedWorkloadDeploymentApplying &&
				deployment.State != GeneratedWorkloadDeploymentIndeterminate {
				return GeneratedWorkloadDeploymentRecord{}, observation,
					fmt.Errorf("%w: clean rollback cannot finalize from %s", ErrGeneratedWorkloadDeploymentState, deployment.State)
			}
			if deployment.Current.StepAttempt != authority.Attempt ||
				deployment.Current.WorkerID != authority.WorkerID {
				return GeneratedWorkloadDeploymentRecord{}, observation,
					fmt.Errorf("%w: clean rollback executor differs", ErrGeneratedWorkloadDeploymentConflict)
			}
			if _, err := tx.Exec(ctx, `
				UPDATE generated_workload_deployments
				SET status='rolled_back',terminal_code=$2,terminal_detail_sha256=$3,
				    current_step_attempt=$4,current_worker_id=$5,terminal_at=clock_timestamp(),
				    updated_at=GREATEST(clock_timestamp(),updated_at+INTERVAL '1 microsecond')
				WHERE id=$1
			`, operationID, terminal.Code, terminal.DetailSHA256,
				authority.Attempt, authority.WorkerID); err != nil {
				return GeneratedWorkloadDeploymentRecord{}, observation,
					fmt.Errorf("finalize clean deployment rollback: %w", err)
			}
			if err := releaseGeneratedWorkloadProjectDeploymentCandidateTx(
				ctx, tx, command, operationID, deployment.Current, true,
			); err != nil {
				return GeneratedWorkloadDeploymentRecord{}, observation, err
			}
		}
	} else if deployment.State == GeneratedWorkloadDeploymentApplying {
		if _, err := tx.Exec(ctx, `
			UPDATE generated_workload_deployments
			SET status='indeterminate',terminal_code='rollback_residual',
			    terminal_detail_sha256=$2,terminal_at=NULL,
			    updated_at=GREATEST(clock_timestamp(),updated_at+INTERVAL '1 microsecond')
			WHERE id=$1
		`, operationID, observation.Observation.SHA256); err != nil {
			return GeneratedWorkloadDeploymentRecord{}, observation,
				fmt.Errorf("fence residual deployment rollback: %w", err)
		}
	} else if deployment.State != GeneratedWorkloadDeploymentIndeterminate {
		return GeneratedWorkloadDeploymentRecord{}, observation,
			fmt.Errorf("%w: residual rollback cannot close from %s", ErrGeneratedWorkloadDeploymentState, deployment.State)
	}
	updated, err := lockGeneratedDeploymentTx(ctx, tx, operationID)
	if err != nil {
		return GeneratedWorkloadDeploymentRecord{}, observation, err
	}
	if err := tx.Commit(ctx); err != nil {
		return GeneratedWorkloadDeploymentRecord{}, observation,
			fmt.Errorf("commit rollback observation and terminal state: %w", err)
	}
	return updated, observation, nil
}

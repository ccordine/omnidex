package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const generatedWorkloadDeploymentRollbackObservationSource = "generated_workload_deployment_rollback_observation"

var generatedDeploymentRollbackVolume = regexp.MustCompile(
	`^[A-Za-z0-9][A-Za-z0-9_.-]{0,254}$`,
)

func (r *Repository) RecordGeneratedWorkloadDeploymentRollbackObservation(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	command GeneratedWorkloadDeploymentCommand,
	plan GeneratedWorkloadDeploymentRollbackPlan,
	rollbackStepAttempt int64,
	observation GeneratedWorkloadDeploymentRollbackObservation,
	terminal GeneratedWorkloadDeploymentTransition,
) (GeneratedWorkloadDeploymentRecord, GeneratedWorkloadDeploymentRollbackObservationRecord, error) {
	return r.recordGeneratedWorkloadDeploymentRollbackObservation(
		ctx, authority, command, plan, rollbackStepAttempt,
		GeneratedWorkloadDeploymentRollbackObservationCommandAttempt,
		observation, terminal,
	)
}

func (r *Repository) RecordGeneratedWorkloadDeploymentPreRollbackObservation(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	command GeneratedWorkloadDeploymentCommand,
	plan GeneratedWorkloadDeploymentRollbackPlan,
	observation GeneratedWorkloadDeploymentRollbackObservation,
	terminal GeneratedWorkloadDeploymentTransition,
) (GeneratedWorkloadDeploymentRecord, GeneratedWorkloadDeploymentRollbackObservationRecord, error) {
	return r.recordGeneratedWorkloadDeploymentRollbackObservation(
		ctx, authority, command, plan, -authority.Attempt,
		GeneratedWorkloadDeploymentRollbackObservationPreAttempt,
		observation, terminal,
	)
}

func (r *Repository) recordGeneratedWorkloadDeploymentRollbackObservation(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	command GeneratedWorkloadDeploymentCommand,
	plan GeneratedWorkloadDeploymentRollbackPlan,
	rollbackStepAttempt int64,
	basis string,
	observation GeneratedWorkloadDeploymentRollbackObservation,
	terminal GeneratedWorkloadDeploymentTransition,
) (GeneratedWorkloadDeploymentRecord, GeneratedWorkloadDeploymentRollbackObservationRecord, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return GeneratedWorkloadDeploymentRecord{}, GeneratedWorkloadDeploymentRollbackObservationRecord{},
			fmt.Errorf("record rollback observation requires PostgreSQL and context")
	}
	if err := validateGeneratedWorkloadDeploymentRollbackPlan(command, plan); err != nil {
		return GeneratedWorkloadDeploymentRecord{}, GeneratedWorkloadDeploymentRollbackObservationRecord{}, err
	}
	if terminal.State != GeneratedWorkloadDeploymentRolledBack {
		return GeneratedWorkloadDeploymentRecord{}, GeneratedWorkloadDeploymentRollbackObservationRecord{},
			fmt.Errorf("rollback observation requires exact rolled-back terminal detail")
	}
	if err := validateGeneratedWorkloadDeploymentTransition(terminal); err != nil {
		return GeneratedWorkloadDeploymentRecord{}, GeneratedWorkloadDeploymentRollbackObservationRecord{}, err
	}
	observationJSON, outcome, err := canonicalGeneratedDeploymentRollbackObservation(plan, observation)
	if err != nil {
		return GeneratedWorkloadDeploymentRecord{}, GeneratedWorkloadDeploymentRollbackObservationRecord{}, err
	}
	identity, err := generatedWorkloadDeploymentOperation(command)
	if err != nil {
		return GeneratedWorkloadDeploymentRecord{}, GeneratedWorkloadDeploymentRollbackObservationRecord{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return GeneratedWorkloadDeploymentRecord{}, GeneratedWorkloadDeploymentRollbackObservationRecord{}, err
	}
	defer tx.Rollback(ctx)
	if err := lockGeneratedDeploymentAuthorityTx(ctx, tx, authority, command); err != nil {
		return GeneratedWorkloadDeploymentRecord{}, GeneratedWorkloadDeploymentRollbackObservationRecord{}, err
	}
	deployment, err := lockGeneratedDeploymentTx(ctx, tx, identity.OperationID)
	if err != nil {
		return GeneratedWorkloadDeploymentRecord{}, GeneratedWorkloadDeploymentRollbackObservationRecord{}, err
	}
	if err := requireGeneratedDeploymentIdentity(deployment, identity); err != nil {
		return GeneratedWorkloadDeploymentRecord{}, GeneratedWorkloadDeploymentRollbackObservationRecord{}, err
	}
	persistedPlan, found, err := loadGeneratedDeploymentRollbackPlanTx(ctx, tx, identity.OperationID, true)
	if err != nil || !found || !equalGeneratedWorkloadDeploymentRollbackPlans(persistedPlan, plan) {
		return GeneratedWorkloadDeploymentRecord{}, GeneratedWorkloadDeploymentRollbackObservationRecord{},
			fmt.Errorf("rollback observation plan differs from durable authority: %w", err)
	}
	if basis == GeneratedWorkloadDeploymentRollbackObservationCommandAttempt {
		attempt, found, err := loadGeneratedDeploymentRollbackAttemptTx(
			ctx, tx, command, identity.OperationID, rollbackStepAttempt, plan, true,
		)
		if err != nil || !found {
			return GeneratedWorkloadDeploymentRecord{}, GeneratedWorkloadDeploymentRollbackObservationRecord{},
				fmt.Errorf("rollback observation has no durable side-effect attempt: %w", err)
		}
		if attempt.CommandSHA256 != plan.Execution.CommandSHA256 ||
			attempt.WorkspaceSHA256 != plan.Execution.WorkspaceSHA256 {
			return GeneratedWorkloadDeploymentRecord{}, GeneratedWorkloadDeploymentRollbackObservationRecord{},
				fmt.Errorf("rollback observation attempt differs from durable plan")
		}
	} else if basis == GeneratedWorkloadDeploymentRollbackObservationPreAttempt {
		var attemptExists, currentObservationExists, cleanupCause, forwardQuiescent bool
		var observationCount int
		queryErr := tx.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_attempts WHERE operation_id=$1),
			       COUNT(*) FILTER(WHERE basis='pre_attempt'),
			       COUNT(*) FILTER(WHERE basis='pre_attempt' AND rollback_step_attempt=$2)>0,
			       EXISTS(SELECT 1 FROM generated_workload_deployment_executions
			              WHERE operation_id=$1),
			       NOT EXISTS(SELECT 1 FROM generated_workload_deployment_executions
			                  WHERE operation_id=$1 AND status='started')
			FROM generated_workload_deployment_rollback_observations WHERE operation_id=$1
		`, identity.OperationID, rollbackStepAttempt).Scan(
			&attemptExists, &observationCount, &currentObservationExists, &cleanupCause,
			&forwardQuiescent,
		)
		if queryErr != nil {
			return GeneratedWorkloadDeploymentRecord{}, GeneratedWorkloadDeploymentRollbackObservationRecord{},
				fmt.Errorf("inspect pre-attempt rollback authority: %w", queryErr)
		}
		capacityAllowsObservation := observationCount < plan.MaxAttempts ||
			outcome == GeneratedWorkloadDeploymentRollbackClean && forwardQuiescent
		if rollbackStepAttempt != -authority.Attempt || attemptExists || !cleanupCause ||
			!capacityAllowsObservation && !currentObservationExists {
			return GeneratedWorkloadDeploymentRecord{}, GeneratedWorkloadDeploymentRollbackObservationRecord{},
				fmt.Errorf("pre-attempt residual rollback observation authority is exhausted")
		}
	} else {
		return GeneratedWorkloadDeploymentRecord{}, GeneratedWorkloadDeploymentRollbackObservationRecord{},
			fmt.Errorf("rollback observation basis is invalid")
	}
	existing, found, err := loadGeneratedDeploymentRollbackObservationTx(
		ctx, tx, identity.OperationID, rollbackStepAttempt, true,
	)
	if err != nil {
		return GeneratedWorkloadDeploymentRecord{}, GeneratedWorkloadDeploymentRollbackObservationRecord{}, err
	}
	if found {
		if existing.Outcome != outcome || existing.Observation.SHA256 != observation.SHA256 {
			return GeneratedWorkloadDeploymentRecord{}, GeneratedWorkloadDeploymentRollbackObservationRecord{},
				fmt.Errorf("%w: rollback observation replay differs", ErrGeneratedWorkloadDeploymentConflict)
		}
		return r.finishGeneratedDeploymentRollbackObservationTx(
			ctx, tx, authority, command, identity.OperationID, deployment, existing, terminal,
		)
	}
	payload, err := generatedDeploymentRollbackObservationEvidence(
		command, identity.OperationID, rollbackStepAttempt, basis,
		observationJSON, outcome, observation,
	)
	if err != nil {
		return GeneratedWorkloadDeploymentRecord{}, GeneratedWorkloadDeploymentRollbackObservationRecord{}, err
	}
	var evidenceID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO evidence(job_id,step_id,kind,source_type,source_ref,payload_json)
		VALUES($1,$2,$3,$4,$5,$6::jsonb) RETURNING id
	`, command.Authority.JobID, command.Authority.StepID, evidence.KindDeploymentObservation,
		generatedWorkloadDeploymentRollbackObservationSource, identity.OperationID, payload,
	).Scan(&evidenceID); err != nil {
		return GeneratedWorkloadDeploymentRecord{}, GeneratedWorkloadDeploymentRollbackObservationRecord{},
			fmt.Errorf("insert rollback observation evidence: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO generated_workload_deployment_rollback_observations(
		 operation_id,rollback_step_attempt,observer_job_id,observer_generation,
		 observer_step_id,observer_step_attempt,observer_worker_id,basis,outcome,
		 observation_json,observation_sha256,evidence_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`, identity.OperationID, rollbackStepAttempt, authority.JobID, authority.Generation,
		authority.StepID, authority.Attempt, authority.WorkerID, basis, outcome,
		observationJSON, observation.SHA256, evidenceID)
	if err != nil {
		return GeneratedWorkloadDeploymentRecord{}, GeneratedWorkloadDeploymentRollbackObservationRecord{},
			fmt.Errorf("insert rollback observation: %w", err)
	}
	recorded, _, err := loadGeneratedDeploymentRollbackObservationTx(
		ctx, tx, identity.OperationID, rollbackStepAttempt, true,
	)
	if err != nil {
		return GeneratedWorkloadDeploymentRecord{}, GeneratedWorkloadDeploymentRollbackObservationRecord{}, err
	}
	return r.finishGeneratedDeploymentRollbackObservationTx(
		ctx, tx, authority, command, identity.OperationID, deployment, recorded, terminal,
	)
}

func generatedDeploymentRollbackObservationEvidence(
	command GeneratedWorkloadDeploymentCommand,
	operationID string,
	rollbackStepAttempt int64,
	basis string,
	observationJSON, outcome string,
	observation GeneratedWorkloadDeploymentRollbackObservation,
) (string, error) {
	summary := "Observed exact project-owned resources after a bounded deployment rollback command attempt."
	if basis == GeneratedWorkloadDeploymentRollbackObservationPreAttempt {
		summary = "Observed exact project-owned resources before any deployment rollback command attempt."
	}
	record := evidence.Record{
		JobID: command.Authority.JobID, StepID: command.Authority.StepID,
		Kind:       evidence.KindDeploymentObservation,
		SourceType: generatedWorkloadDeploymentRollbackObservationSource,
		SourceRef:  operationID, Excerpt: observationJSON,
		Summary: summary,
		Hash:    observation.SHA256, Confidence: 1,
		Metadata: map[string]any{
			"outcome": outcome, "basis": basis,
			"rollback_step_attempt": rollbackStepAttempt,
			"postcondition_sha256":  observation.PostconditionSHA256,
		},
	}
	if err := record.Validate(); err != nil {
		return "", err
	}
	payload, err := json.Marshal(record)
	if err != nil || len(payload) > 1<<20 {
		return "", fmt.Errorf("encode rollback observation evidence: %w", err)
	}
	return string(payload), nil
}

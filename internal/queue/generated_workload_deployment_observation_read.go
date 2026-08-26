package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func loadGeneratedDeploymentObservationTx(
	ctx context.Context,
	querier generatedDeploymentExecutionQuerier,
	operationID string,
	ordinal int,
	lock bool,
) (GeneratedWorkloadDeploymentObservationRecord, bool, error) {
	query := `
		SELECT operation_id,slot_name,slot_ordinal,command_evidence_id,
		       observation_json,observation_sha256,services_sha256,endpoint_sha256,
		       evidence_id,created_at
		FROM generated_workload_deployment_observations
		WHERE operation_id=$1 AND slot_ordinal=$2`
	if lock {
		query += ` FOR UPDATE`
	}
	var record GeneratedWorkloadDeploymentObservationRecord
	var observationJSON string
	var observationSHA, servicesSHA, endpointSHA string
	err := querier.QueryRow(ctx, query, operationID, ordinal).Scan(
		&record.OperationID, &record.Slot.Name, &record.Slot.Ordinal,
		&record.CommandEvidenceID, &observationJSON, &observationSHA, &servicesSHA,
		&endpointSHA, &record.EvidenceID, &record.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return GeneratedWorkloadDeploymentObservationRecord{}, false, nil
	}
	if err != nil {
		return GeneratedWorkloadDeploymentObservationRecord{}, false, fmt.Errorf("load deployment observation: %w", err)
	}
	if err := decodeExactGeneratedDeploymentJSON(observationJSON, &record.Observation); err != nil {
		return GeneratedWorkloadDeploymentObservationRecord{}, false, fmt.Errorf("decode deployment observation: %w", err)
	}
	canonical, err := canonicalGeneratedDeploymentJSON(record.Observation)
	if err != nil {
		return GeneratedWorkloadDeploymentObservationRecord{}, false, fmt.Errorf("canonicalize durable deployment observation: %w", err)
	}
	if canonical != observationJSON {
		return GeneratedWorkloadDeploymentObservationRecord{}, false, fmt.Errorf("durable deployment observation is not canonical")
	}
	if record.Slot != GeneratedDeploymentSlotInitialObserve &&
		record.Slot != GeneratedDeploymentSlotFinalObserve ||
		record.EvidenceID <= 0 || record.CommandEvidenceID <= 0 ||
		record.Observation.SHA256 != observationSHA ||
		record.Observation.ServicesSHA256 != servicesSHA ||
		record.Observation.EndpointSHA256 != endpointSHA {
		return GeneratedWorkloadDeploymentObservationRecord{}, false, fmt.Errorf("durable deployment observation is incomplete")
	}
	return record, true, nil
}

func (r *Repository) GeneratedWorkloadDeploymentEvidence(
	ctx context.Context,
	jobID, generation int64,
) (*GeneratedWorkloadDeploymentEvidenceSnapshot, error) {
	verification, err := r.BoundGeneratedWorkloadVerification(ctx, jobID, generation)
	if err != nil || verification == nil {
		return nil, err
	}
	deployment, err := r.CurrentGeneratedWorkloadDeployment(ctx, jobID, generation)
	if err != nil || deployment == nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT slot_ordinal FROM generated_workload_deployment_executions
		WHERE operation_id=$1 ORDER BY slot_ordinal
	`, deployment.Record.OperationID)
	if err != nil {
		return nil, err
	}
	var ordinals []int
	for rows.Next() {
		var ordinal int
		if err := rows.Scan(&ordinal); err != nil {
			rows.Close()
			return nil, err
		}
		ordinals = append(ordinals, ordinal)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	binding, found, err := loadGeneratedDeploymentVerificationBindingTx(
		ctx, r.pool, deployment.Record.OperationID, false,
	)
	if err != nil || !found {
		return nil, fmt.Errorf("load deployment evidence binding: %w", err)
	}
	rollback, rollbackFound, err := loadGeneratedDeploymentRollbackPlanTx(
		ctx, r.pool, deployment.Record.OperationID, false,
	)
	if err != nil {
		return nil, err
	}
	if !rollbackFound && deployment.Record.State != GeneratedWorkloadDeploymentApplied &&
		deployment.Record.State != GeneratedWorkloadDeploymentFailed &&
		deployment.Record.State != GeneratedWorkloadDeploymentRolledBack {
		return nil, fmt.Errorf("load deployment rollback plan: nonterminal deployment has no exact plan")
	}
	manifestJSON, manifestSHA, err := canonicalGeneratedDeploymentLifecycleManifest(
		deployment.Command, binding.LifecycleManifest,
	)
	if err != nil || manifestJSON == "" || manifestSHA != binding.LifecycleManifestSHA256 {
		return nil, fmt.Errorf("validate deployment evidence binding: %w", err)
	}
	result := &GeneratedWorkloadDeploymentEvidenceSnapshot{
		Verification: *verification, Binding: binding,
	}
	if rollbackFound {
		result.RollbackPlan = &rollback
	}
	for _, ordinal := range ordinals {
		record, found, err := loadGeneratedDeploymentExecutionTx(
			ctx, r.pool, deployment.Record.OperationID, ordinal, false,
		)
		if err != nil || !found {
			return nil, fmt.Errorf("load deployment execution %d: %w", ordinal, err)
		}
		result.Executions = append(result.Executions, record)
		if record.Slot == GeneratedDeploymentSlotInitialObserve ||
			record.Slot == GeneratedDeploymentSlotFinalObserve {
			observation, observed, err := loadGeneratedDeploymentObservationTx(
				ctx, r.pool, deployment.Record.OperationID, ordinal, false,
			)
			if err != nil {
				return nil, err
			}
			if observed {
				canonical, err := validateAndCanonicalizeGeneratedDeploymentObservation(
					deployment.Command, observation.Observation,
				)
				if err != nil || canonical == "" || observation.OperationID != deployment.Record.OperationID ||
					observation.CommandEvidenceID != record.EvidenceID {
					return nil, fmt.Errorf("validate recovered deployment observation %s: %w", record.Slot.Name, err)
				}
				result.Observations = append(result.Observations, observation)
			}
		}
	}
	return result, nil
}

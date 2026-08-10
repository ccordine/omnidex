package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

type lifecycleOperationRecord struct {
	ID                 LifecycleOperationID
	JobID              int64
	ObservedGeneration int64
	ResultGeneration   int64
	StepID             *int64
	StepContextID      *int64
	Kind               LifecycleOperationKind
	CommandSHA256      string
	ResultJobStatus    string
	ResultStepStatus   *string
	ResultJob          model.Job
}

func loadLifecycleOperationTx(
	ctx context.Context,
	tx pgx.Tx,
	descriptor lifecycleOperationDescriptor,
	expectedJobID int64,
) (lifecycleOperationRecord, bool, error) {
	identityCreated, err := reserveLifecycleOperationIdentityTx(
		ctx, tx, descriptor.ID, descriptor.Kind, descriptor.SHA256, descriptor.Payload,
	)
	if err != nil {
		return lifecycleOperationRecord{}, false, err
	}
	var record lifecycleOperationRecord
	var resultJobJSON []byte
	var payloadMatches bool
	err = tx.QueryRow(ctx, `
		SELECT operation_id, job_id, observed_generation, result_generation,
		       step_id, step_context_id, kind, command_sha256,
		       result_job_status, result_step_status, result_job,
		       command_payload = $2::jsonb
		FROM job_lifecycle_operations
		WHERE operation_id=$1
	`, descriptor.ID, string(descriptor.Payload)).Scan(
		&record.ID, &record.JobID, &record.ObservedGeneration, &record.ResultGeneration,
		&record.StepID, &record.StepContextID, &record.Kind, &record.CommandSHA256,
		&record.ResultJobStatus, &record.ResultStepStatus, &resultJobJSON, &payloadMatches,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		if !identityCreated {
			return lifecycleOperationRecord{}, false, fmt.Errorf(
				"lifecycle operation %q is registered without its immutable result",
				descriptor.ID,
			)
		}
		return lifecycleOperationRecord{}, false, nil
	}
	if err != nil {
		return lifecycleOperationRecord{}, false, fmt.Errorf("read lifecycle operation %q: %w", descriptor.ID, err)
	}
	if record.JobID != expectedJobID || record.Kind != descriptor.Kind ||
		record.CommandSHA256 != descriptor.SHA256 || !payloadMatches {
		return lifecycleOperationRecord{}, false, fmt.Errorf(
			"%w: operation ID %q is persisted with different job, kind, or command content",
			ErrLifecycleOperationConflict, descriptor.ID,
		)
	}
	if err := json.Unmarshal(resultJobJSON, &record.ResultJob); err != nil {
		return lifecycleOperationRecord{}, false, fmt.Errorf(
			"decode lifecycle operation %q result: %w", descriptor.ID, err,
		)
	}
	if record.ResultJob.ID != record.JobID ||
		record.ResultJob.CurrentGeneration != record.ResultGeneration ||
		record.ResultJob.Status != record.ResultJobStatus {
		return lifecycleOperationRecord{}, false, fmt.Errorf(
			"lifecycle operation %q contains inconsistent result authority", descriptor.ID,
		)
	}
	return record, true, nil
}

func insertLifecycleOperationTx(
	ctx context.Context,
	tx pgx.Tx,
	descriptor lifecycleOperationDescriptor,
	record lifecycleOperationRecord,
) error {
	resultJobJSON, err := json.Marshal(record.ResultJob)
	if err != nil {
		return fmt.Errorf("encode lifecycle operation result: %w", err)
	}
	result, err := tx.Exec(ctx, `
		INSERT INTO job_lifecycle_operations (
			operation_id, job_id, observed_generation, result_generation,
			step_id, step_context_id, kind, command_sha256, command_payload,
			result_job_status, result_step_status, result_job
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11,$12::jsonb)
		ON CONFLICT (operation_id) DO NOTHING
	`, descriptor.ID, record.JobID, record.ObservedGeneration, record.ResultGeneration,
		record.StepID, record.StepContextID, descriptor.Kind, descriptor.SHA256,
		string(descriptor.Payload), record.ResultJobStatus, record.ResultStepStatus, string(resultJobJSON))
	if err != nil {
		return fmt.Errorf("record lifecycle operation %q: %w", descriptor.ID, err)
	}
	if result.RowsAffected() == 1 {
		return nil
	}
	if _, found, err := loadLifecycleOperationTx(ctx, tx, descriptor, record.JobID); err != nil {
		return err
	} else if !found {
		return fmt.Errorf("lifecycle operation %q conflicted but could not be loaded", descriptor.ID)
	}
	return nil
}

func scanLockedJobTx(ctx context.Context, tx pgx.Tx, jobID int64) (model.Job, error) {
	return scanJob(tx.QueryRow(ctx, `
		SELECT id, instruction, pipeline, status, result, error, metadata,
		       current_generation, created_at, updated_at, completed_at
		FROM jobs WHERE id=$1
	`, jobID))
}

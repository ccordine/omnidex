package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func requireReplanReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	record lifecycleOperationRecord,
	command ReplanJobCommand,
) error {
	return requireGenerationCutoverReplayTx(
		ctx,
		tx,
		record,
		command,
		LifecycleReplanJob,
		jobGenerationPurposeReplan,
		model.JobStatusRunning,
	)
}

func requireInterruptReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	record lifecycleOperationRecord,
	command ReplanJobCommand,
) error {
	if record.ResultGeneration == record.ObservedGeneration {
		if record.JobID != command.JobID || record.Kind != LifecycleInterruptJob || record.StepID == nil || record.ResultStepStatus == nil || *record.ResultStepStatus != model.StepStatusWaiting || record.ResultJobStatus != model.JobStatusWaiting {
			return lifecycleReplayStateError(record.ID, "frozen execution pause")
		}
		var exists bool
		err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM job_steps WHERE id=$1 AND job_id=$2 AND generation=$3 AND action='v3_coding')`, *record.StepID, record.JobID, record.ResultGeneration).Scan(&exists)
		if err != nil {
			return err
		}
		if !exists {
			return lifecycleReplayStateError(record.ID, "paused execution step")
		}
		return nil
	}
	return requireGenerationCutoverReplayTx(
		ctx,
		tx,
		record,
		command,
		LifecycleInterruptJob,
		jobGenerationPurposeInterrupt,
		model.JobStatusWaiting,
	)
}

func requireGenerationCutoverReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	record lifecycleOperationRecord,
	command ReplanJobCommand,
	expectedKind LifecycleOperationKind,
	expectedPurpose string,
	expectedJobStatus string,
) error {
	if record.JobID != command.JobID || record.Kind != expectedKind ||
		record.StepID != nil || record.ResultStepStatus != nil ||
		record.ResultGeneration != record.ObservedGeneration+1 ||
		record.ResultJobStatus != expectedJobStatus {
		return lifecycleReplayStateError(record.ID, expectedPurpose+" generation result")
	}
	var predecessor int64
	var purpose, boundary, feedback string
	if err := tx.QueryRow(ctx, `
		SELECT predecessor_generation, purpose, boundary_action, feedback
		FROM job_generations WHERE job_id=$1 AND generation=$2
		FOR UPDATE
	`, record.JobID, record.ResultGeneration).Scan(
		&predecessor, &purpose, &boundary, &feedback,
	); err != nil {
		return fmt.Errorf("validate lifecycle %s generation: %w", expectedPurpose, err)
	}
	if predecessor != record.ObservedGeneration || purpose != expectedPurpose ||
		(boundary != replanCodingBoundary && boundary != replanObjectiveBoundary) ||
		feedback != command.Feedback {
		return lifecycleReplayStateError(record.ID, "immutable "+expectedPurpose+" generation")
	}
	return nil
}

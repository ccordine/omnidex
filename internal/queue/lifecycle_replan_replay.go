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
	feedbackSHA string,
) error {
	return requireGenerationCutoverReplayTx(
		ctx,
		tx,
		record,
		command,
		feedbackSHA,
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
	feedbackSHA string,
) error {
	return requireGenerationCutoverReplayTx(
		ctx,
		tx,
		record,
		command,
		feedbackSHA,
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
	feedbackSHA string,
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
	var purpose, boundary, feedback, persistedSHA string
	if err := tx.QueryRow(ctx, `
		SELECT predecessor_generation, purpose, boundary_action, feedback, feedback_sha256
		FROM job_generations WHERE job_id=$1 AND generation=$2
		FOR UPDATE
	`, record.JobID, record.ResultGeneration).Scan(
		&predecessor, &purpose, &boundary, &feedback, &persistedSHA,
	); err != nil {
		return fmt.Errorf("validate lifecycle %s generation: %w", expectedPurpose, err)
	}
	if predecessor != record.ObservedGeneration || purpose != expectedPurpose ||
		(boundary != replanCodingBoundary && boundary != replanObjectiveBoundary) ||
		feedback != command.Feedback || persistedSHA != feedbackSHA {
		return lifecycleReplayStateError(record.ID, "immutable "+expectedPurpose+" generation")
	}
	return nil
}

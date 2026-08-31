package queue

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func requireReplanReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	record lifecycleOperationRecord,
	command ReplanJobCommand,
	feedbackSHA string,
) error {
	if record.JobID != command.JobID || record.StepID != nil || record.StepContextID != nil ||
		record.ResultGeneration != record.ObservedGeneration+1 {
		return lifecycleReplayStateError(record.ID, "replan generation result")
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
		return fmt.Errorf("validate lifecycle replan generation: %w", err)
	}
	if predecessor != record.ObservedGeneration || purpose != jobGenerationPurposeReplan ||
		(boundary != replanCodingBoundary && boundary != replanObjectiveBoundary) ||
		feedback != command.Feedback || persistedSHA != feedbackSHA {
		return lifecycleReplayStateError(record.ID, "immutable replan generation")
	}
	return nil
}

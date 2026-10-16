package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func requireCompleteStepReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	record lifecycleOperationRecord,
	command CompleteStepCommand,
) error {
	if record.StepID == nil || *record.StepID != command.StepID ||
		record.ResultStepStatus == nil || *record.ResultStepStatus != model.StepStatusCompleted {
		return lifecycleReplayStateError(record.ID, "completed step result")
	}
	err := requireLifecycleStepTx(
		ctx, tx, record, model.StepStatusCompleted, &command.Output, lifecycleExpectedText(""),
	)
	if err != nil {
		return err
	}
	if err := requireTerminalObjectiveCodingTailCanceledTx(ctx, tx, record, command); err != nil {
		return err
	}
	return requireTerminalLifecycleAuthorityTx(ctx, tx, record, command.Output, "")
}

func requireFailStepReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	record lifecycleOperationRecord,
	command FailStepCommand,
) error {
	if record.StepID == nil || *record.StepID != command.StepID ||
		record.ResultStepStatus == nil || *record.ResultStepStatus != model.StepStatusFailed {
		return lifecycleReplayStateError(record.ID, "failed step result")
	}
	if err := requireLifecycleStepTx(
		ctx, tx, record, model.StepStatusFailed, nil, &command.Error,
	); err != nil {
		return err
	}
	return requireTerminalLifecycleAuthorityTx(ctx, tx, record, "", command.Error)
}

func requireSubmitFeedbackReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	record lifecycleOperationRecord,
	command SubmitJobFeedbackCommand,
) error {
	if command.ExpectedGeneration != nil && *command.ExpectedGeneration != record.ObservedGeneration {
		return lifecycleReplayStateError(record.ID, "feedback observed generation")
	}
	if record.StepID == nil || record.ResultStepStatus == nil {
		return lifecycleReplayStateError(record.ID, "feedback step result")
	}
	if record.ResultGeneration > record.ObservedGeneration {
		var bound bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM evidence reply
		  JOIN evidence source ON source.job_id=reply.job_id AND source.step_id=reply.step_id
		    AND source.kind='coding_agent' AND source.payload_json->>'phase' IN ('task_incomplete','planning_incomplete')
		    AND source.payload_json->>'call_id'=reply.payload_json->>'call_id'
		  WHERE reply.job_id=$1 AND reply.kind='coding_agent' AND reply.payload_json->>'generation'=$2::bigint::text
		    AND reply.payload_json->>'phase'='execution_reply' AND reply.payload_json->'notes'->>0=$3 AND reply.payload_json->'notes'->>1=$4)`,
			record.JobID, record.ObservedGeneration, command.OperationID, command.Feedback).Scan(&bound); err != nil {
			return err
		}
		if !bound {
			return lifecycleReplayStateError(record.ID, "continuation owner reply")
		}
		return nil
	}
	if *record.ResultStepStatus == model.StepStatusRunning {
		return requireCodingExecutionReplyTx(ctx, tx, record.JobID, *record.StepID, record.ObservedGeneration, command.OperationID, command.Feedback)
	}
	if *record.ResultStepStatus != model.StepStatusCompleted {
		return lifecycleReplayStateError(record.ID, "feedback step result")
	}
	if err := requireLifecycleStepTx(
		ctx, tx, record, model.StepStatusCompleted, &command.Feedback, lifecycleExpectedText(""),
	); err != nil {
		return err
	}
	return requireTerminalLifecycleAuthorityTx(ctx, tx, record, command.Feedback, "")
}

func requireLifecycleStepTx(
	ctx context.Context,
	tx pgx.Tx,
	record lifecycleOperationRecord,
	wantStatus string,
	wantOutput, wantError *string,
) error {
	var generation int64
	var supersededAt *int64
	var status, output, stepError string
	err := tx.QueryRow(ctx, `
		SELECT generation, superseded_at_generation, status,
		       COALESCE(output, ''), COALESCE(error, '')
		FROM job_steps WHERE id=$1 AND job_id=$2
		FOR UPDATE
	`, *record.StepID, record.JobID).Scan(&generation, &supersededAt, &status, &output, &stepError)
	if err != nil {
		return fmt.Errorf("validate lifecycle step %d: %w", *record.StepID, err)
	}
	if generation != record.ObservedGeneration || status != wantStatus ||
		(wantOutput != nil && output != *wantOutput) ||
		(wantError != nil && stepError != *wantError) {
		return lifecycleReplayStateError(record.ID, "step authority")
	}
	if supersededAt != nil && *supersededAt <= generation {
		return lifecycleReplayStateError(record.ID, "step generation retirement")
	}
	return nil
}

func lifecycleExpectedText(value string) *string {
	return &value
}

func requireTerminalLifecycleAuthorityTx(
	ctx context.Context,
	tx pgx.Tx,
	record lifecycleOperationRecord,
	_, _ string,
) error {
	return requireLifecycleGenerationExistsTx(ctx, tx, record.JobID, record.ObservedGeneration)
}

func requireLifecycleGenerationExistsTx(ctx context.Context, tx pgx.Tx, jobID, generation int64) error {
	var found int64
	if err := tx.QueryRow(ctx, `
		SELECT generation FROM job_generations WHERE job_id=$1 AND generation=$2
	`, jobID, generation).Scan(&found); err != nil {
		return fmt.Errorf("validate lifecycle generation %d for job %d: %w", generation, jobID, err)
	}
	return nil
}

func lifecycleReplayStateError(id model.LifecycleOperationID, subject string) error {
	return fmt.Errorf("%w: lifecycle operation %q has inconsistent %s", ErrStepNotWritable, id, subject)
}

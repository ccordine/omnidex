package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
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
	if record.StepContextID != nil {
		if err := requireLifecycleStepContextTx(
			ctx, tx, *record.StepContextID, command.StepID, command.ContextKey, command.ContextValue,
		); err != nil {
			return err
		}
	} else if command.ContextKey != "" {
		return lifecycleReplayStateError(record.ID, "missing step context")
	}
	return requireTerminalLifecycleAuthorityTx(ctx, tx, record, command.Output, "")
}

func requireFailStepReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	record lifecycleOperationRecord,
	command FailStepCommand,
) error {
	if record.StepID == nil || *record.StepID != command.StepID || record.StepContextID != nil ||
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
	if record.StepID == nil || record.StepContextID == nil ||
		record.ResultStepStatus == nil || *record.ResultStepStatus != model.StepStatusCompleted {
		return lifecycleReplayStateError(record.ID, "feedback step result")
	}
	if err := requireLifecycleStepTx(
		ctx, tx, record, model.StepStatusCompleted, &command.Feedback, lifecycleExpectedText(""),
	); err != nil {
		return err
	}
	if err := requireLifecycleStepContextTx(
		ctx, tx, *record.StepContextID, *record.StepID, "user_feedback", command.Feedback,
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
	proofContent, failureReason string,
) error {
	if record.ResultJobStatus != model.JobStatusCompleted && record.ResultJobStatus != model.JobStatusFailed {
		return requireLifecycleGenerationExistsTx(ctx, tx, record.JobID, record.ObservedGeneration)
	}
	header, root, err := loadInitialTaskRootTx(ctx, tx, record.JobID, record.ObservedGeneration)
	if err != nil {
		return err
	}
	wantRoot, wantLedger := taskstate.NodeDone, taskstate.LedgerClosed
	reason := ""
	if record.ResultJobStatus == model.JobStatusFailed {
		wantRoot, wantLedger, reason = taskstate.NodeFailed, taskstate.LedgerFailed, failureReason
	}
	if root.Status != wantRoot || root.StatusReason != reason || header.Status != wantLedger {
		return lifecycleReplayStateError(record.ID, "terminal task state")
	}
	if err := requireCanonicalRootTransitionEventTx(
		ctx, tx, header, record.ObservedGeneration, *record.StepID,
		wantRoot, proofContent, reason,
	); err != nil {
		return err
	}
	terminalReason := "job completed after every current-generation step completed"
	if record.Kind == LifecycleSubmitFeedback {
		terminalReason = "job completed after its final waiting step received user feedback"
	}
	if record.ResultJobStatus == model.JobStatusFailed {
		terminalReason = "job failed after its current-generation step failed"
	}
	return requireCanonicalLedgerCloseEventTx(
		ctx, tx, header, record.ObservedGeneration, wantLedger, record.StepID, terminalReason,
	)
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

func requireLifecycleStepContextTx(
	ctx context.Context,
	tx pgx.Tx,
	contextID, stepID int64,
	wantKey, wantValue string,
) error {
	var persistedStepID int64
	var key, value string
	if err := tx.QueryRow(ctx, `
		SELECT step_id, key, value FROM step_contexts WHERE id=$1
	`, contextID).Scan(&persistedStepID, &key, &value); err != nil {
		return fmt.Errorf("validate lifecycle step context %d: %w", contextID, err)
	}
	if persistedStepID != stepID || key != wantKey || value != wantValue {
		return fmt.Errorf("lifecycle step context %d has inconsistent authority", contextID)
	}
	return nil
}

func lifecycleReplayStateError(id LifecycleOperationID, subject string) error {
	return fmt.Errorf("%w: lifecycle operation %q has inconsistent %s", taskstate.ErrInvalidState, id, subject)
}

package queue

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func validateTaskCommandJobLifecycle(header taskLedgerHeader, command taskstate.Command) error {
	closeCommand, closesLedger, err := taskLedgerCloseCommand(command)
	if err != nil {
		return err
	}
	wantLedgerStatus, jobIsTerminal := taskLedgerStatusForJob(header.JobStatus)
	if !jobIsTerminal {
		if header.Status != taskstate.LedgerActive {
			return fmt.Errorf(
				"%w: nonterminal job %d has terminal task ledger status %q",
				taskstate.ErrInvalidState, header.Owner.JobID, header.Status,
			)
		}
		if closesLedger {
			return fmt.Errorf(
				"%w: task ledger terminalization requires a matching terminal job transition",
				taskstate.ErrInvalidCommand,
			)
		}
		return nil
	}
	if header.Status == wantLedgerStatus {
		return fmt.Errorf(
			"%w: terminal task ledger for job %d accepts only an exact command replay",
			taskstate.ErrInvalidState, header.Owner.JobID,
		)
	}
	if header.Status != taskstate.LedgerActive {
		return fmt.Errorf(
			"%w: job %d status %q disagrees with task ledger status %q",
			taskstate.ErrInvalidState, header.Owner.JobID, header.JobStatus, header.Status,
		)
	}
	if !closesLedger || closeCommand.Status != wantLedgerStatus {
		return fmt.Errorf(
			"%w: terminal job %d status %q requires task ledger status %q",
			taskstate.ErrInvalidState, header.Owner.JobID, header.JobStatus, wantLedgerStatus,
		)
	}
	return nil
}

func terminalizeTaskLedgerTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID, observedGeneration int64,
	jobStatus string,
	stepID *int64,
	reason string,
) error {
	wantStatus, ok := taskLedgerStatusForJob(jobStatus)
	if !ok {
		return fmt.Errorf("%w: job status %q is not terminal", taskstate.ErrInvalidCommand, jobStatus)
	}
	header, err := loadTaskLedgerHeaderTx(ctx, tx, jobID, true)
	if err != nil {
		return err
	}
	if header.Generation != observedGeneration || header.JobStatus != jobStatus {
		return fmt.Errorf(
			"%w: job %d terminal authority changed from generation %d status %q to generation %d status %q",
			ErrStaleJobGeneration, jobID, observedGeneration, jobStatus, header.Generation, header.JobStatus,
		)
	}
	if header.Status == wantStatus {
		return nil
	}
	if header.Status != taskstate.LedgerActive {
		return fmt.Errorf(
			"%w: job %d status %q disagrees with task ledger status %q",
			taskstate.ErrInvalidState, jobID, jobStatus, header.Status,
		)
	}
	commandID, err := terminalTaskCommandID(jobID, observedGeneration, wantStatus)
	if err != nil {
		return err
	}
	_, err = applyQueueOwnedTaskCommandTx(ctx, tx, jobID, observedGeneration, taskstate.CloseLedgerCommand{
		CommandID: commandID, ExpectedVersion: header.Version, Actor: taskstate.AuthorityCode,
		Status: wantStatus, StepID: stepID, Reason: reason,
	})
	return err
}

func terminalTaskCommandID(
	jobID, generation int64,
	status taskstate.LedgerStatus,
) (taskstate.CommandID, error) {
	return taskstate.NewCommandID(
		"task-ledger-terminal-v1",
		strconv.FormatInt(jobID, 10),
		strconv.FormatInt(generation, 10),
		string(status),
	)
}

func currentJobGenerationTx(ctx context.Context, tx pgx.Tx, jobID int64) (int64, error) {
	var generation int64
	if err := tx.QueryRow(ctx, `SELECT current_generation FROM jobs WHERE id=$1`, jobID).Scan(&generation); err != nil {
		return 0, fmt.Errorf("read job %d current generation: %w", jobID, err)
	}
	if generation <= 0 {
		return 0, fmt.Errorf("%w: job %d has invalid generation %d", ErrInvalidJobGeneration, jobID, generation)
	}
	return generation, nil
}

func taskLedgerStatusForJob(jobStatus string) (taskstate.LedgerStatus, bool) {
	switch jobStatus {
	case model.JobStatusCompleted:
		return taskstate.LedgerClosed, true
	case model.JobStatusFailed:
		return taskstate.LedgerFailed, true
	case model.JobStatusCanceled:
		return taskstate.LedgerCanceled, true
	default:
		return "", false
	}
}

func taskLedgerCloseCommand(command taskstate.Command) (taskstate.CloseLedgerCommand, bool, error) {
	switch typed := command.(type) {
	case taskstate.CloseLedgerCommand:
		return typed, true, nil
	case *taskstate.CloseLedgerCommand:
		if typed == nil {
			return taskstate.CloseLedgerCommand{}, false, fmt.Errorf("%w: nil close-ledger command", taskstate.ErrInvalidCommand)
		}
		return *typed, true, nil
	default:
		return taskstate.CloseLedgerCommand{}, false, nil
	}
}

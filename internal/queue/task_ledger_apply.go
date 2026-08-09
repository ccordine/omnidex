package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ApplyTaskCommand(
	ctx context.Context,
	jobID int64,
	observedGeneration int64,
	command taskstate.Command,
) (taskstate.Event, error) {
	if err := validateTaskLedgerRequest(r, ctx, jobID); err != nil {
		return taskstate.Event{}, err
	}
	if observedGeneration <= 0 {
		return taskstate.Event{}, fmt.Errorf("task command observed generation must be positive")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return taskstate.Event{}, fmt.Errorf("begin task command for job %d: %w", jobID, err)
	}
	defer tx.Rollback(ctx)
	event, err := applyTaskCommandTx(ctx, tx, jobID, observedGeneration, command)
	if err != nil {
		return taskstate.Event{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return taskstate.Event{}, fmt.Errorf("commit task command: %w", err)
	}
	return event, nil
}

func applyTaskCommandTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID int64,
	observedGeneration int64,
	command taskstate.Command,
) (taskstate.Event, error) {
	return applyTaskCommandWithBoundaryTx(
		ctx, tx, jobID, observedGeneration, command, generalTaskCommandBoundary{},
	)
}

func applyQueueOwnedTaskCommandTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID int64,
	observedGeneration int64,
	command taskstate.Command,
) (taskstate.Event, error) {
	return applyTaskCommandWithBoundaryTx(
		ctx, tx, jobID, observedGeneration, command, queueOwnedTaskCommandBoundary{},
	)
}

func applyTaskCommandWithBoundaryTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID int64,
	observedGeneration int64,
	command taskstate.Command,
	boundary taskCommandBoundary,
) (taskstate.Event, error) {
	descriptor, err := taskstate.DescribeCommand(command)
	if err != nil {
		return taskstate.Event{}, err
	}
	if err := boundary.validate(command); err != nil {
		return taskstate.Event{}, err
	}
	if observedGeneration <= 0 {
		return taskstate.Event{}, fmt.Errorf("task command observed generation must be positive")
	}
	if descriptor.ExpectedVersion > math.MaxInt64 {
		return taskstate.Event{}, fmt.Errorf("%w: task command expected version exceeds PostgreSQL BIGINT", taskstate.ErrInvalidCommand)
	}
	header, err := loadTaskLedgerHeaderTx(ctx, tx, jobID, true)
	if err != nil {
		return taskstate.Event{}, err
	}
	if existing, found, err := loadTaskEventByCommandTx(
		ctx, tx, header, observedGeneration, descriptor,
	); err != nil {
		return taskstate.Event{}, err
	} else if found {
		return existing, nil
	}
	if header.Generation != observedGeneration {
		return taskstate.Event{}, fmt.Errorf(
			"%w: task command observed job %d generation %d, current generation is %d",
			ErrStaleJobGeneration, jobID, observedGeneration, header.Generation,
		)
	}
	if err := validateTaskCommandJobLifecycle(header, command); err != nil {
		return taskstate.Event{}, err
	}
	if err := validateTaskCommandGenerationTx(ctx, tx, jobID, command); err != nil {
		return taskstate.Event{}, err
	}
	if header.Version >= math.MaxInt64 {
		return taskstate.Event{}, fmt.Errorf("%w: task ledger %q exhausted PostgreSQL version capacity", taskstate.ErrInvalidState, header.ID)
	}
	ledger, err := restoreTaskLedgerTx(ctx, tx, header)
	if err != nil {
		return taskstate.Event{}, err
	}
	event, err := ledger.Apply(command)
	if err != nil {
		return taskstate.Event{}, err
	}
	if event.CommandID != descriptor.ID || event.CommandSHA256 != descriptor.SHA256 ||
		event.CommandKind != descriptor.Kind || event.Authority != descriptor.Actor ||
		event.Version != header.Version+1 {
		return taskstate.Event{}, fmt.Errorf("%w: task command %q produced inconsistent event identity", taskstate.ErrInvalidState, descriptor.ID)
	}
	state := ledger.MaterializedState()
	if err := taskstate.ValidateMaterializedState(state); err != nil {
		return taskstate.Event{}, err
	}
	result, err := tx.Exec(ctx, `
		UPDATE task_ledgers
		SET version=$3, status=$4,
		    closed_at=CASE WHEN $4='active' THEN NULL ELSE NOW() END,
		    updated_at=NOW()
		WHERE id=$1 AND job_id=$2 AND version=$5 AND status='active'
	`, header.ID, jobID, int64(event.Version), state.Status, int64(descriptor.ExpectedVersion))
	if err != nil {
		return taskstate.Event{}, fmt.Errorf("advance task ledger %q version: %w", header.ID, err)
	}
	if result.RowsAffected() != 1 {
		return taskstate.Event{}, fmt.Errorf("%w: task ledger %q version changed during command %q", taskstate.ErrVersionConflict, header.ID, descriptor.ID)
	}
	if err := persistTaskLedgerMutation(ctx, tx, header.ID, jobID, event, state); err != nil {
		return taskstate.Event{}, err
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return taskstate.Event{}, fmt.Errorf("encode task event for command %q: %w", descriptor.ID, err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO task_events (
			ledger_id, job_id, job_generation, ledger_version, command_id, command_sha256,
			command_kind, event_kind, actor, step_id, payload
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::jsonb)
	`, header.ID, jobID, observedGeneration, int64(event.Version), event.CommandID, event.CommandSHA256,
		event.CommandKind, event.Kind, event.Authority, event.StepID, payload); err != nil {
		return taskstate.Event{}, fmt.Errorf("append task event for command %q: %w", descriptor.ID, err)
	}
	return event, nil
}

func loadTaskEventByCommandTx(
	ctx context.Context,
	tx pgx.Tx,
	header taskLedgerHeader,
	observedGeneration int64,
	descriptor taskstate.CommandDescriptor,
) (taskstate.Event, bool, error) {
	var version, persistedGeneration int64
	var commandSHA, commandKind, eventKind, actor string
	var stepID *int64
	var payload []byte
	err := tx.QueryRow(ctx, `
		SELECT job_generation, ledger_version, command_sha256, command_kind, event_kind, actor, step_id, payload
		FROM task_events
		WHERE ledger_id=$1 AND command_id=$2
	`, header.ID, descriptor.ID).Scan(
		&persistedGeneration, &version, &commandSHA, &commandKind, &eventKind, &actor, &stepID, &payload,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return taskstate.Event{}, false, nil
	}
	if err != nil {
		return taskstate.Event{}, false, fmt.Errorf("read task event for command %q: %w", descriptor.ID, err)
	}
	if commandSHA != descriptor.SHA256 {
		return taskstate.Event{}, false, fmt.Errorf(
			"%w: command ID %q is persisted with different content", taskstate.ErrCommandIDConflict, descriptor.ID,
		)
	}
	if persistedGeneration != observedGeneration {
		return taskstate.Event{}, false, fmt.Errorf(
			"%w: command ID %q is persisted for generation %d, received %d",
			taskstate.ErrCommandIDConflict, descriptor.ID, persistedGeneration, observedGeneration,
		)
	}
	event, err := decodeTaskEventColumns(
		header.ID,
		version,
		descriptor.ID,
		commandSHA,
		taskstate.CommandKind(commandKind),
		taskstate.EventKind(eventKind),
		taskstate.Authority(actor),
		stepID,
		payload,
	)
	if err != nil {
		return taskstate.Event{}, false, fmt.Errorf("decode immutable task event for command %q: %w", descriptor.ID, err)
	}
	if event.CommandKind != descriptor.Kind || event.Authority != descriptor.Actor ||
		event.Version != descriptor.ExpectedVersion+1 {
		return taskstate.Event{}, false, fmt.Errorf(
			"%w: immutable task event for command %q disagrees with its command descriptor",
			taskstate.ErrInvalidState,
			descriptor.ID,
		)
	}
	if event.Version > header.Version {
		return taskstate.Event{}, false, fmt.Errorf(
			"%w: immutable task event for command %q is ahead of the ledger version", taskstate.ErrInvalidState, descriptor.ID,
		)
	}
	return event, true, nil
}

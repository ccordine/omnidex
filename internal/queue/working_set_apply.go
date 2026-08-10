package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/workingset"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ApplyWorkingSetCommand(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	command workingset.Command,
) (workingset.Event, error) {
	if err := validateWorkingSetRequest(r, ctx, authority.JobID, authority.Generation); err != nil {
		return workingset.Event{}, err
	}
	descriptor, err := workingset.DescribeCommand(command)
	if err != nil {
		return workingset.Event{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return workingset.Event{}, fmt.Errorf("begin working-set command: %w", err)
	}
	defer tx.Rollback(ctx)
	event, err := applyWorkingSetCommandTx(ctx, tx, authority, command, descriptor)
	if err != nil {
		return workingset.Event{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return workingset.Event{}, fmt.Errorf("commit working-set command %q: %w", descriptor.ID, err)
	}
	return event, nil
}

func loadWorkingSetEventByCommandTx(
	ctx context.Context,
	tx pgx.Tx,
	commandID workingset.CommandID,
) (WorkingSetEventRecord, bool, error) {
	var record WorkingSetEventRecord
	var setID, persistedCommandID, commandSHA, commandKind, eventKind, actor string
	var reacquiredItemID *string
	var reacquisitionCount *int64
	var version int64
	var payload []byte
	err := tx.QueryRow(ctx, `
		SELECT id, working_set_id, job_id, generation, working_set_version,
		       command_id, command_sha256, command_kind, event_kind, actor,
		       reacquired_item_id, reacquisition_count, payload, created_at
		FROM working_set_events WHERE command_id=$1
	`, commandID).Scan(
		&record.ID, &setID, &record.JobID, &record.Generation, &version,
		&persistedCommandID, &commandSHA, &commandKind, &eventKind, &actor,
		&reacquiredItemID, &reacquisitionCount, &payload, &record.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return WorkingSetEventRecord{}, false, nil
	}
	if err != nil {
		return WorkingSetEventRecord{}, false, fmt.Errorf("read working-set command %q: %w", commandID, err)
	}
	record.Event, err = decodeWorkingSetEventColumns(
		workingset.SetID(setID), version, workingset.CommandID(persistedCommandID), commandSHA,
		workingset.CommandKind(commandKind), workingset.EventKind(eventKind), actor,
		reacquiredItemID, reacquisitionCount, payload,
	)
	if err != nil {
		return WorkingSetEventRecord{}, false, fmt.Errorf("decode working-set command %q: %w", commandID, err)
	}
	return record, true, nil
}

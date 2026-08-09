package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/workingset"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ApplyWorkingSetCommand(
	ctx context.Context,
	jobID, observedGeneration int64,
	command workingset.Command,
) (workingset.Event, error) {
	if err := validateWorkingSetRequest(r, ctx, jobID, observedGeneration); err != nil {
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
	ledger, err := loadTaskLedgerHeaderTx(ctx, tx, jobID, true)
	if err != nil {
		return workingset.Event{}, err
	}
	expectedSetID, err := workingset.NewSetID(workingset.Owner{
		LedgerID: ledger.ID, JobID: jobID, Generation: observedGeneration,
	})
	if err != nil {
		return workingset.Event{}, err
	}
	if existing, found, err := loadWorkingSetEventByCommandTx(ctx, tx, descriptor.ID); err != nil {
		return workingset.Event{}, err
	} else if found {
		if existing.JobID != jobID || existing.Generation != observedGeneration ||
			existing.Event.SetID != expectedSetID || existing.Event.CommandSHA256 != descriptor.SHA256 ||
			existing.Event.CommandKind != descriptor.Kind || existing.Event.Actor != descriptor.Actor {
			return workingset.Event{}, fmt.Errorf(
				"%w: command ID %q is persisted with different authority or content",
				workingset.ErrCommandIDConflict, descriptor.ID,
			)
		}
		return existing.Event, nil
	}
	if err := requireCurrentWorkingSetAuthority(ledger, observedGeneration); err != nil {
		return workingset.Event{}, err
	}
	before, err := loadWorkingSetSnapshotTx(ctx, tx, ledger, observedGeneration, true)
	if err != nil {
		return workingset.Event{}, err
	}
	set, err := workingset.Restore(before)
	if err != nil {
		return workingset.Event{}, err
	}
	event, err := set.Apply(command)
	if err != nil {
		return workingset.Event{}, err
	}
	after := set.Snapshot()
	if event.SetID != before.ID || event.Version != after.Version || after.Version != before.Version+1 {
		return workingset.Event{}, fmt.Errorf(
			"%w: command %q produced an inconsistent materialized version",
			workingset.ErrInvalidSet, descriptor.ID,
		)
	}
	if err := persistWorkingSetDiffTx(ctx, tx, before, after); err != nil {
		return workingset.Event{}, err
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return workingset.Event{}, fmt.Errorf("encode working-set event for command %q: %w", descriptor.ID, err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO working_set_events (
			working_set_id, job_id, generation, working_set_version,
			command_id, command_sha256, command_kind, event_kind, actor, payload
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::json)
	`, event.SetID, jobID, observedGeneration, int64(event.Version), event.CommandID,
		event.CommandSHA256, event.CommandKind, event.Kind, event.Actor, payload,
	); err != nil {
		return workingset.Event{}, fmt.Errorf("append working-set event for command %q: %w", descriptor.ID, err)
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
	var version int64
	var payload []byte
	err := tx.QueryRow(ctx, `
		SELECT id, working_set_id, job_id, generation, working_set_version,
		       command_id, command_sha256, command_kind, event_kind, actor, payload, created_at
		FROM working_set_events WHERE command_id=$1
	`, commandID).Scan(
		&record.ID, &setID, &record.JobID, &record.Generation, &version,
		&persistedCommandID, &commandSHA, &commandKind, &eventKind, &actor, &payload, &record.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return WorkingSetEventRecord{}, false, nil
	}
	if err != nil {
		return WorkingSetEventRecord{}, false, fmt.Errorf("read working-set command %q: %w", commandID, err)
	}
	record.Event, err = decodeWorkingSetEventColumns(
		workingset.SetID(setID), version, workingset.CommandID(persistedCommandID), commandSHA,
		workingset.CommandKind(commandKind), workingset.EventKind(eventKind), actor, payload,
	)
	if err != nil {
		return WorkingSetEventRecord{}, false, fmt.Errorf("decode working-set command %q: %w", commandID, err)
	}
	return record, true, nil
}

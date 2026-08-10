package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/workingset"
	"github.com/jackc/pgx/v5"
)

func applyWorkingSetCommandTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	command workingset.Command,
	descriptor workingset.CommandDescriptor,
) (workingset.Event, error) {
	jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, authority)
	if err != nil {
		return workingset.Event{}, err
	}
	if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
		return workingset.Event{}, staleStepAttemptError(authority, "working-set writer is not running", nil)
	}
	return applyAuthorizedWorkingSetCommandTx(
		ctx, tx, authority.JobID, authority.Generation, command, descriptor,
	)
}

func applyAuthorizedWorkingSetCommandTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID, generation int64,
	command workingset.Command,
	descriptor workingset.CommandDescriptor,
) (workingset.Event, error) {
	if jobID <= 0 || generation <= 0 {
		return workingset.Event{}, fmt.Errorf("working-set command requires exact owner authority")
	}
	ledger, err := loadTaskLedgerHeaderTx(ctx, tx, jobID, false)
	if err != nil {
		return workingset.Event{}, err
	}
	expectedSetID, err := workingset.NewSetID(workingset.Owner{
		LedgerID: ledger.ID, JobID: jobID, Generation: generation,
	})
	if err != nil {
		return workingset.Event{}, err
	}
	if existing, found, err := loadWorkingSetEventByCommandTx(ctx, tx, descriptor.ID); err != nil {
		return workingset.Event{}, err
	} else if found {
		if existing.JobID != jobID || existing.Generation != generation ||
			existing.Event.SetID != expectedSetID || existing.Event.CommandSHA256 != descriptor.SHA256 ||
			existing.Event.CommandKind != descriptor.Kind || existing.Event.Actor != descriptor.Actor {
			return workingset.Event{}, fmt.Errorf(
				"%w: command ID %q is persisted with different authority or content",
				workingset.ErrCommandIDConflict, descriptor.ID,
			)
		}
		return existing.Event, nil
	}
	if err := requireCurrentWorkingSetAuthority(ledger, generation); err != nil {
		return workingset.Event{}, err
	}
	before, err := loadWorkingSetSnapshotTx(ctx, tx, ledger, generation, true)
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
	reacquiredItemID, reacquisitionCount := workingSetEventReacquisitionColumns(event)
	if _, err := tx.Exec(ctx, `
		INSERT INTO working_set_events (
			working_set_id,job_id,generation,working_set_version,
			command_id,command_sha256,command_kind,event_kind,actor,
			reacquired_item_id,reacquisition_count,payload
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::json)
	`, event.SetID, jobID, generation, int64(event.Version), event.CommandID,
		event.CommandSHA256, event.CommandKind, event.Kind, event.Actor,
		reacquiredItemID, reacquisitionCount, payload); err != nil {
		return workingset.Event{}, fmt.Errorf("append working-set event for command %q: %w", descriptor.ID, err)
	}
	return event, nil
}

func workingSetEventReacquisitionColumns(event workingset.Event) (any, any) {
	if event.Reacquisition == nil {
		return nil, nil
	}
	return event.Reacquisition.ItemID, int64(event.Reacquisition.Count)
}

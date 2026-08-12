package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func loadTaskLedgerAtVersionTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID int64,
	version uint64,
) (taskstate.MaterializedState, error) {
	header, err := loadTaskLedgerHeaderTx(ctx, tx, jobID, false)
	if err != nil {
		return taskstate.MaterializedState{}, err
	}
	if version == 0 || version > header.Version {
		return taskstate.MaterializedState{}, fmt.Errorf(
			"%w: proposal materialization ledger version %d is outside current version %d",
			ErrCognitionConflict, version, header.Version,
		)
	}
	rows, err := tx.Query(ctx, `
		SELECT ledger_version,command_id,command_sha256,command_kind,event_kind,actor,step_id,payload
		FROM task_events WHERE ledger_id=$1 AND ledger_version<=$2 ORDER BY ledger_version
	`, header.ID, int64(version))
	if err != nil {
		return taskstate.MaterializedState{}, err
	}
	defer rows.Close()
	events := make([]taskstate.Event, 0)
	for rows.Next() {
		var (
			persistedVersion                   int64
			commandID, commandSHA, commandKind string
			eventKind, actor                   string
			stepID                             *int64
			payload                            []byte
		)
		if err := rows.Scan(
			&persistedVersion, &commandID, &commandSHA, &commandKind,
			&eventKind, &actor, &stepID, &payload,
		); err != nil {
			return taskstate.MaterializedState{}, err
		}
		event, err := decodeTaskEventColumns(
			header.ID, persistedVersion, taskstate.CommandID(commandID), commandSHA,
			taskstate.CommandKind(commandKind), taskstate.EventKind(eventKind),
			taskstate.Authority(actor), stepID, payload,
		)
		if err != nil {
			return taskstate.MaterializedState{}, fmt.Errorf(
				"decode proposal materialization ledger event %d: %w", persistedVersion, err,
			)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return taskstate.MaterializedState{}, err
	}
	if uint64(len(events)) != version {
		return taskstate.MaterializedState{}, fmt.Errorf(
			"%w: proposal materialization ledger prefix has %d events for version %d",
			ErrCognitionConflict, len(events), version,
		)
	}
	ledger, err := taskstate.Reconstruct(header.ID, header.Owner, events)
	if err != nil {
		return taskstate.MaterializedState{}, fmt.Errorf("reconstruct proposal materialization ledger: %w", err)
	}
	state := ledger.MaterializedState()
	if state.Version != version {
		return taskstate.MaterializedState{}, fmt.Errorf(
			"%w: proposal materialization ledger prefix ended at version %d",
			ErrCognitionConflict, state.Version,
		)
	}
	return state, nil
}

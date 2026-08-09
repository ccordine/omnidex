package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
	"github.com/jackc/pgx/v5"
)

const maxWorkingSetEventPageSize = 100

type WorkingSetEventRecord struct {
	ID         int64
	JobID      int64
	Generation int64
	Event      workingset.Event
	CreatedAt  time.Time
}

func (r *Repository) ListWorkingSetEvents(
	ctx context.Context,
	jobID, generation, afterID int64,
	limit int,
) ([]WorkingSetEventRecord, error) {
	if afterID < 0 {
		return nil, fmt.Errorf("working-set event cursor must not be negative")
	}
	if limit < 1 || limit > maxWorkingSetEventPageSize {
		return nil, fmt.Errorf(
			"working-set event page limit must be between 1 and %d", maxWorkingSetEventPageSize,
		)
	}
	if err := validateWorkingSetRequest(r, ctx, jobID, generation); err != nil {
		return nil, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return nil, fmt.Errorf("begin working-set event page: %w", err)
	}
	defer tx.Rollback(ctx)
	ledger, err := loadTaskLedgerHeaderTx(ctx, tx, jobID, false)
	if err != nil {
		return nil, err
	}
	if generation > ledger.Generation {
		return nil, fmt.Errorf(
			"%w: job %d has no generation %d", ErrInvalidJobGeneration, jobID, generation,
		)
	}
	snapshot, err := loadWorkingSetSnapshotTx(ctx, tx, ledger, generation, false)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id, working_set_version, command_id, command_sha256,
		       command_kind, event_kind, actor, payload, created_at
		FROM working_set_events
		WHERE working_set_id=$1 AND job_id=$2 AND generation=$3 AND id>$4
		ORDER BY id ASC LIMIT $5
	`, snapshot.ID, jobID, generation, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("read working-set event page for job %d: %w", jobID, err)
	}
	defer rows.Close()
	records := make([]WorkingSetEventRecord, 0, limit)
	for rows.Next() {
		var record WorkingSetEventRecord
		var version int64
		var commandID, commandSHA, commandKind, eventKind, actor string
		var payload []byte
		if err := rows.Scan(
			&record.ID, &version, &commandID, &commandSHA,
			&commandKind, &eventKind, &actor, &payload, &record.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan working-set event page for job %d: %w", jobID, err)
		}
		record.JobID, record.Generation = jobID, generation
		record.Event, err = decodeWorkingSetEventColumns(
			snapshot.ID, version, workingset.CommandID(commandID), commandSHA,
			workingset.CommandKind(commandKind), workingset.EventKind(eventKind), actor, payload,
		)
		if err != nil {
			return nil, fmt.Errorf("decode working-set event %d: %w", record.ID, err)
		}
		if record.ID <= afterID || record.CreatedAt.IsZero() || record.Event.Version > snapshot.Version {
			return nil, fmt.Errorf("%w: working-set event page contains invalid authority", workingset.ErrInvalidSet)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate working-set event page for job %d: %w", jobID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit working-set event page for job %d: %w", jobID, err)
	}
	return records, nil
}

func decodeWorkingSetEventColumns(
	setID workingset.SetID,
	version int64,
	commandID workingset.CommandID,
	commandSHA string,
	commandKind workingset.CommandKind,
	eventKind workingset.EventKind,
	actor string,
	payload []byte,
) (workingset.Event, error) {
	var event workingset.Event
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return workingset.Event{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return workingset.Event{}, fmt.Errorf("event payload contains trailing JSON")
	}
	if version <= 0 || event.SetID != setID || event.Version != uint64(version) ||
		event.CommandID != commandID || event.CommandSHA256 != commandSHA ||
		event.CommandKind != commandKind || event.Kind != eventKind ||
		event.Actor != taskstate.Authority(actor) {
		return workingset.Event{}, fmt.Errorf(
			"%w: immutable working-set event disagrees with its typed columns", workingset.ErrInvalidSet,
		)
	}
	if err := workingset.ValidateEvent(event); err != nil {
		return workingset.Event{}, err
	}
	return event, nil
}

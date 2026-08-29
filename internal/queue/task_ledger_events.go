package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"time"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

const maxTaskEventPageSize = 100

type TaskEventRecord struct {
	ID        int64
	Event     taskstate.Event
	CreatedAt time.Time
}

func (r *Repository) ListTaskEvents(
	ctx context.Context,
	jobID int64,
	afterID int64,
	limit int,
) ([]TaskEventRecord, error) {
	if afterID < 0 {
		return nil, fmt.Errorf("task event cursor must not be negative")
	}
	if limit < 1 || limit > maxTaskEventPageSize {
		return nil, fmt.Errorf("task event page limit must be between 1 and %d", maxTaskEventPageSize)
	}
	if err := validateTaskLedgerRequest(r, ctx, jobID); err != nil {
		return nil, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return nil, fmt.Errorf("begin task event page: %w", err)
	}
	defer tx.Rollback(ctx)
	header, err := loadTaskLedgerHeaderTx(ctx, tx, jobID, false)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id, ledger_version, command_id, command_sha256, command_kind,
		       event_kind, actor, step_id, payload, created_at
		FROM task_events
		WHERE ledger_id=$1 AND job_id=$2 AND id>$3
		ORDER BY id ASC
		LIMIT $4
	`, header.ID, jobID, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("read task event page for job %d: %w", jobID, err)
	}
	defer rows.Close()
	records := make([]TaskEventRecord, 0, limit)
	for rows.Next() {
		var record TaskEventRecord
		var version int64
		var commandID, commandSHA, commandKind, eventKind, actor string
		var stepID *int64
		var payload []byte
		if err := rows.Scan(
			&record.ID, &version, &commandID, &commandSHA, &commandKind,
			&eventKind, &actor, &stepID, &payload, &record.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan task event page for job %d: %w", jobID, err)
		}
		record.Event, err = decodeTaskEventColumns(
			header.ID, version, taskstate.CommandID(commandID), commandSHA,
			taskstate.CommandKind(commandKind), taskstate.EventKind(eventKind),
			taskstate.Authority(actor), stepID, payload,
		)
		if err != nil {
			return nil, fmt.Errorf("decode task event %d: %w", record.ID, err)
		}
		if record.ID <= 0 || record.CreatedAt.IsZero() {
			return nil, fmt.Errorf("%w: task event page contains invalid record identity", taskstate.ErrInvalidState)
		}
		if record.Event.Version > header.Version {
			return nil, fmt.Errorf("%w: task event %d is ahead of ledger version", taskstate.ErrInvalidState, record.ID)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task event page for job %d: %w", jobID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit task event page for job %d: %w", jobID, err)
	}
	return records, nil
}

func decodeTaskEventColumns(
	ledgerID taskstate.LedgerID,
	version int64,
	commandID taskstate.CommandID,
	commandSHA string,
	commandKind taskstate.CommandKind,
	eventKind taskstate.EventKind,
	actor taskstate.Authority,
	stepID *int64,
	payload []byte,
) (taskstate.Event, error) {
	var event taskstate.Event
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return event, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		return taskstate.Event{}, fmt.Errorf("event payload has trailing content: %w", err)
	}
	if version <= 0 || event.LedgerID != ledgerID || event.Version != uint64(version) ||
		event.CommandID != commandID || event.CommandSHA256 != commandSHA ||
		event.CommandKind != commandKind || event.Kind != eventKind ||
		event.Authority != actor || !reflect.DeepEqual(event.StepID, stepID) {
		return taskstate.Event{}, fmt.Errorf(
			"%w: immutable task event disagrees with its typed columns", taskstate.ErrInvalidState,
		)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return taskstate.Event{}, fmt.Errorf("decode event payload fields: %w", err)
	}
	_, payloadHasStep := fields["step_id"]
	if payloadHasStep != (stepID != nil) {
		return taskstate.Event{}, fmt.Errorf(
			"%w: immutable task event step presence disagrees with its typed column", taskstate.ErrInvalidState,
		)
	}
	if err := validateTaskEventShape(event, fields); err != nil {
		return taskstate.Event{}, err
	}
	if event.NodeIDs == nil {
		event.NodeIDs = make([]taskstate.NodeID, 0)
	}
	if event.VerificationRefs == nil {
		event.VerificationRefs = make([]taskstate.Ref, 0)
	}
	return event, nil
}

func validateTaskEventShape(event taskstate.Event, fields map[string]json.RawMessage) error {
	if err := taskstate.ValidateEventProjection(event); err != nil {
		return err
	}
	if fields == nil {
		payload, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("encode task event shape: %w", err)
		}
		fields = make(map[string]json.RawMessage)
		if err := json.Unmarshal(payload, &fields); err != nil {
			return fmt.Errorf("decode task event shape fields: %w", err)
		}
	}
	allowed, required := taskEventJSONFieldContract(event.Kind)
	if fields != nil {
		for key := range fields {
			if !allowed[key] {
				return fmt.Errorf("%w: task event %q contains forbidden field %q", taskstate.ErrInvalidState, event.Kind, key)
			}
		}
		for key := range required {
			if _, exists := fields[key]; !exists {
				return fmt.Errorf("%w: task event %q is missing field %q", taskstate.ErrInvalidState, event.Kind, key)
			}
		}
	}
	return nil
}

func taskEventJSONFieldContract(kind taskstate.EventKind) (map[string]bool, map[string]bool) {
	allowed := taskEventFieldSet(
		"ledger_id", "ledger_version", "command_id", "command_sha256",
		"command_kind", "event_kind", "actor",
	)
	required := taskEventFieldSet(
		"ledger_id", "ledger_version", "command_id", "command_sha256",
		"command_kind", "event_kind", "actor",
	)
	allow := func(names ...string) {
		for _, name := range names {
			allowed[name] = true
		}
	}
	require := func(names ...string) {
		allow(names...)
		for _, name := range names {
			required[name] = true
		}
	}
	switch kind {
	case taskstate.EventNodeAdded:
		require("node")
		allow("step_id")
	case taskstate.EventEdgeAdded:
		require("edge")
	case taskstate.EventEntryAdded:
		require("entry")
		allow("step_id")
	case taskstate.EventEntryRejected:
		require("entry_id", "reason")
		allow("verification_refs")
	case taskstate.EventEntryResolved:
		require("entry_id", "verification_refs", "reason")
	case taskstate.EventEntrySuperseded:
		require("entry_id", "replacement_id", "reason")
	case taskstate.EventNodesReadied:
		require("node_ids")
	case taskstate.EventNodeStepAssigned:
		require("node_id", "step_id")
	case taskstate.EventNodeTransitioned:
		require("node_id", "from_status", "to_status")
		allow("step_id", "verification_refs", "reason")
	case taskstate.EventNodeGenerationSuperseded:
		require("node_ids", "retiring_generation", "superseded_at_generation", "reason")
	case taskstate.EventLedgerClosed:
		require("ledger_status", "reason")
		allow("step_id")
	}
	return allowed, required
}

func taskEventFieldSet(names ...string) map[string]bool {
	result := make(map[string]bool, len(names))
	for _, name := range names {
		result[name] = true
	}
	return result
}

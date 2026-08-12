package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/workingset"
	"github.com/jackc/pgx/v5"
)

func loadCognitionWorkingSetHistoryTx(
	ctx context.Context,
	tx pgx.Tx,
	episode CognitionEpisode,
	finalVersion uint64,
) (cognitionWorkingSetHistory, error) {
	header, err := loadTaskLedgerHeaderTx(ctx, tx, episode.Authority.JobID, false)
	if err != nil {
		return cognitionWorkingSetHistory{}, err
	}
	current, err := loadWorkingSetSnapshotTx(ctx, tx, header, episode.Authority.Generation, false)
	if err != nil {
		return cognitionWorkingSetHistory{}, err
	}
	if current.ID != workingset.SetID(episode.WorkingSetID) || current.Version < finalVersion {
		return cognitionWorkingSetHistory{}, fmt.Errorf("%w: sealed Working Set version is unavailable", ErrCognitionConflict)
	}
	events, created, err := loadCognitionWorkingSetEventsTx(ctx, tx, current, finalVersion)
	if err != nil {
		return cognitionWorkingSetHistory{}, err
	}
	baselineCount := 0
	for baselineCount < len(created) && created[baselineCount].Before(episode.CreatedAt) {
		baselineCount++
	}
	baselineSet, err := workingset.Reconstruct(current.Owner, current.Budget, events[:baselineCount])
	if err != nil {
		return cognitionWorkingSetHistory{}, fmt.Errorf("reconstruct episode-start Working Set: %w", err)
	}
	finalSet, err := workingset.Reconstruct(current.Owner, current.Budget, events)
	if err != nil {
		return cognitionWorkingSetHistory{}, fmt.Errorf("reconstruct sealed Working Set: %w", err)
	}
	finalCapturedAt := episode.CreatedAt
	if len(created) > 0 {
		finalCapturedAt = created[len(created)-1]
	}
	history := cognitionWorkingSetHistory{
		Start: CognitionTraceWorkingSetSnapshot{
			Schema: CognitionTraceWorkingSetSnapshotSchemaV1, Point: "episode_start",
			CapturedAt: episode.CreatedAt, Snapshot: baselineSet.Snapshot(),
		},
		Final: CognitionTraceWorkingSetSnapshot{
			Schema: CognitionTraceWorkingSetSnapshotSchemaV1, Point: "terminal",
			CapturedAt: finalCapturedAt, Snapshot: finalSet.Snapshot(),
		},
	}
	for index := baselineCount; index < len(events); index++ {
		history.Events = append(history.Events, CognitionTraceWorkingSetEvent{
			Schema:    CognitionTraceWorkingSetEventSchemaV1,
			CreatedAt: created[index], Event: events[index],
		})
	}
	if len(history.Events) == 0 {
		history.Events = []CognitionTraceWorkingSetEvent{}
	}
	if err := validateCognitionWorkingSetHistory(history, finalVersion); err != nil {
		return cognitionWorkingSetHistory{}, err
	}
	return history, nil
}

func loadCognitionWorkingSetEventsTx(
	ctx context.Context,
	tx pgx.Tx,
	snapshot workingset.Snapshot,
	finalVersion uint64,
) ([]workingset.Event, []time.Time, error) {
	if finalVersion > MaxCognitionTraceRecords {
		return nil, nil, fmt.Errorf("%w: sealed Working Set event stream exceeds hard cap", ErrCognitionConflict)
	}
	rows, err := tx.Query(ctx, `
		SELECT working_set_version,command_id,command_sha256,command_kind,event_kind,actor,
		       reacquired_item_id,reacquisition_count,payload,created_at
		FROM working_set_events
		WHERE working_set_id=$1 AND job_id=$2 AND generation=$3 AND working_set_version<=$4
		ORDER BY working_set_version
	`, snapshot.ID, snapshot.Owner.JobID, snapshot.Owner.Generation, int64(finalVersion))
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	events := make([]workingset.Event, 0, int(finalVersion))
	created := make([]time.Time, 0, int(finalVersion))
	for rows.Next() {
		var version int64
		var commandID, commandSHA, commandKind, eventKind, actor string
		var itemID *string
		var count *int64
		var payload []byte
		var timestamp time.Time
		if err := rows.Scan(&version, &commandID, &commandSHA, &commandKind, &eventKind,
			&actor, &itemID, &count, &payload, &timestamp); err != nil {
			return nil, nil, err
		}
		event, err := decodeWorkingSetEventColumns(
			snapshot.ID, version, workingset.CommandID(commandID), commandSHA,
			workingset.CommandKind(commandKind), workingset.EventKind(eventKind), actor,
			itemID, count, payload,
		)
		if err != nil {
			return nil, nil, err
		}
		events, created = append(events, event), append(created, timestamp)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	if uint64(len(events)) != finalVersion {
		return nil, nil, fmt.Errorf("%w: sealed Working Set event stream has gaps", ErrCognitionConflict)
	}
	return events, created, nil
}

func loadCognitionTraceWorkingSetPayloadTx(
	ctx context.Context,
	tx pgx.Tx,
	episode CognitionEpisode,
	finalVersion uint64,
	record cognitionTraceRecord,
) ([]byte, error) {
	history, err := loadCognitionWorkingSetHistoryTx(ctx, tx, episode, finalVersion)
	if err != nil {
		return nil, err
	}
	var value any
	switch record.Kind {
	case "working_set_snapshot":
		if record.ID == string(history.Start.Snapshot.ID)+":episode-start" {
			value = history.Start
		} else if record.ID == string(history.Final.Snapshot.ID)+":terminal" {
			value = history.Final
		}
	case "working_set_event":
		for _, event := range history.Events {
			if event.Event.Version == uint64(record.Sequence) &&
				record.ID == string(event.Event.SetID)+":event:"+fmt.Sprint(event.Event.Version) {
				value = event
				break
			}
		}
	}
	if value == nil {
		return nil, fmt.Errorf("%w: sealed cognition Working Set payload is unavailable", ErrCognitionConflict)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if cognitionPayloadSHA(raw) != record.SHA256 {
		return nil, fmt.Errorf("%w: sealed cognition Working Set payload changed", ErrCognitionConflict)
	}
	return raw, nil
}

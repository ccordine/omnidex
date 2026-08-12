package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/workingset"
	"github.com/jackc/pgx/v5"
)

const (
	CognitionTraceWorkingSetSnapshotSchemaV1 = "omnidex.cognition-working-set-snapshot-trace.v1"
	CognitionTraceWorkingSetEventSchemaV1    = "omnidex.cognition-working-set-event-trace.v1"
)

type CognitionTraceWorkingSetSnapshot struct {
	Schema     string              `json:"schema"`
	Point      string              `json:"point"`
	CapturedAt time.Time           `json:"captured_at"`
	Snapshot   workingset.Snapshot `json:"snapshot"`
}

type CognitionTraceWorkingSetEvent struct {
	Schema    string           `json:"schema"`
	CreatedAt time.Time        `json:"created_at"`
	Event     workingset.Event `json:"event"`
}

type cognitionWorkingSetHistory struct {
	Start  CognitionTraceWorkingSetSnapshot
	Events []CognitionTraceWorkingSetEvent
	Final  CognitionTraceWorkingSetSnapshot
}

func appendCognitionWorkingSetTraceRecordsTx(
	ctx context.Context,
	tx pgx.Tx,
	episode CognitionEpisode,
	workingVersion uint64,
	records []cognitionTraceRecord,
) ([]cognitionTraceRecord, error) {
	history, err := loadCognitionWorkingSetHistoryTx(ctx, tx, episode, workingVersion)
	if err != nil {
		return nil, err
	}
	startRaw, err := json.Marshal(history.Start)
	if err != nil {
		return nil, err
	}
	setID := string(history.Start.Snapshot.ID)
	records = append(records, cognitionTraceRecord{
		Kind: "working_set_snapshot", Phase: 1, Sequence: int64(history.Start.Snapshot.Version),
		ID: setID + ":episode-start", SHA256: cognitionPayloadSHA(startRaw),
	})
	points, maxOrdinal, err := loadCognitionProjectionWorkingVersionsTx(ctx, tx, episode.EpisodeID)
	if err != nil {
		return nil, err
	}
	for _, event := range history.Events {
		raw, err := json.Marshal(event)
		if err != nil {
			return nil, err
		}
		records = append(records, cognitionTraceRecord{
			Kind: "working_set_event", CallOrdinal: workingSetEventCallOrdinal(event.Event.Version, points, maxOrdinal),
			Phase: 5, Sequence: int64(event.Event.Version),
			ID:     setID + ":event:" + strconv.FormatUint(event.Event.Version, 10),
			SHA256: cognitionPayloadSHA(raw),
		})
	}
	finalRaw, err := json.Marshal(history.Final)
	if err != nil {
		return nil, err
	}
	records = append(records, cognitionTraceRecord{
		Kind: "working_set_snapshot", CallOrdinal: maxOrdinal + 1, Phase: 90,
		Sequence: int64(history.Final.Snapshot.Version), ID: setID + ":terminal",
		SHA256: cognitionPayloadSHA(finalRaw),
	})
	return records, nil
}

type cognitionProjectionWorkingVersion = CognitionProjectionWorkingVersion

func loadCognitionProjectionWorkingVersionsTx(
	ctx context.Context,
	tx pgx.Tx,
	episodeID cognition.EpisodeID,
) ([]cognitionProjectionWorkingVersion, int64, error) {
	rows, err := tx.Query(ctx, `
		SELECT snapshots.call_ordinal,MAX(projections.working_set_version)
		FROM cognition_runtime_snapshots snapshots
		JOIN context_projections projections ON projections.projection_id=snapshots.projection_id
		WHERE snapshots.episode_id=$1
		GROUP BY snapshots.call_ordinal ORDER BY snapshots.call_ordinal
	`, episodeID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	points := make([]cognitionProjectionWorkingVersion, 0)
	var maximum int64
	var priorWorkingVersion int64
	for rows.Next() {
		var point cognitionProjectionWorkingVersion
		if err := rows.Scan(&point.CallOrdinal, &point.WorkingVersion); err != nil {
			return nil, 0, err
		}
		if point.CallOrdinal <= maximum || point.WorkingVersion < priorWorkingVersion {
			return nil, 0, fmt.Errorf("%w: cognition projection chronology changed", ErrCognitionConflict)
		}
		maximum = point.CallOrdinal
		priorWorkingVersion = point.WorkingVersion
		points = append(points, point)
	}
	return points, maximum, rows.Err()
}

func workingSetEventCallOrdinal(
	version uint64,
	points []cognitionProjectionWorkingVersion,
	maximum int64,
) int64 {
	for _, point := range points {
		if uint64(point.WorkingVersion) >= version {
			return point.CallOrdinal
		}
	}
	return maximum + 1
}

func validateCognitionWorkingSetHistory(history cognitionWorkingSetHistory, finalVersion uint64) error {
	if history.Start.Schema != CognitionTraceWorkingSetSnapshotSchemaV1 || history.Start.Point != "episode_start" ||
		history.Final.Schema != CognitionTraceWorkingSetSnapshotSchemaV1 || history.Final.Point != "terminal" ||
		history.Start.CapturedAt.IsZero() || history.Final.CapturedAt.Before(history.Start.CapturedAt) ||
		history.Start.Snapshot.ID != history.Final.Snapshot.ID ||
		history.Final.Snapshot.Version != finalVersion ||
		workingset.ValidateSnapshot(history.Start.Snapshot) != nil ||
		workingset.ValidateSnapshot(history.Final.Snapshot) != nil {
		return fmt.Errorf("%w: cognition Working Set history endpoints are invalid", ErrCognitionConflict)
	}
	for index, event := range history.Events {
		if event.Schema != CognitionTraceWorkingSetEventSchemaV1 || event.CreatedAt.IsZero() ||
			event.Event.Version != history.Start.Snapshot.Version+uint64(index)+1 ||
			workingset.ValidateEvent(event.Event) != nil {
			return fmt.Errorf("%w: cognition Working Set event %d is invalid", ErrCognitionConflict, index)
		}
	}
	replayed, err := workingset.Restore(history.Start.Snapshot)
	if err != nil {
		return fmt.Errorf("%w: restore cognition Working Set baseline: %v", ErrCognitionConflict, err)
	}
	for index, supplied := range history.Events {
		command, err := workingset.DecodeCommand(supplied.Event.CommandKind, supplied.Event.Command)
		if err != nil {
			return err
		}
		actual, err := replayed.Apply(command)
		if err != nil || !reflect.DeepEqual(actual, supplied.Event) {
			return fmt.Errorf("%w: cognition Working Set event %d replay diverged", ErrCognitionConflict, index)
		}
	}
	if !reflect.DeepEqual(replayed.Snapshot(), history.Final.Snapshot) {
		return fmt.Errorf("%w: cognition Working Set event replay diverged", ErrCognitionConflict)
	}
	return nil
}

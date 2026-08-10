package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func loadCognitionPreparedSnapshotBySHATx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	episode CognitionEpisode,
	graph CognitionObligationGraphRecord,
	snapshotSHA string,
) (CognitionRuntimeSnapshotRecord, error) {
	var callOrdinal int64
	var obligationID cognition.ObligationID
	if err := tx.QueryRow(ctx, `
		SELECT call_ordinal,obligation_node_id FROM cognition_runtime_snapshots
		WHERE snapshot_sha256=$1 AND episode_id=$2 AND job_id=$3 AND generation=$4
		  AND step_id=$5 AND actor_attempt=$6 AND actor_worker_id=$7
	`, snapshotSHA, episode.EpisodeID, authority.JobID, authority.Generation, authority.StepID,
		authority.Attempt, authority.WorkerID).Scan(&callOrdinal, &obligationID); err != nil {
		return CognitionRuntimeSnapshotRecord{}, fmt.Errorf("load cognition prepared snapshot identity: %w", err)
	}
	if callOrdinal <= 0 {
		return CognitionRuntimeSnapshotRecord{}, fmt.Errorf("%w: prepared snapshot call ordinal is invalid", ErrCognitionConflict)
	}
	journal, found, err := loadCognitionSnapshotJournalTx(
		ctx, tx, authority, episode, obligationID, uint64(callOrdinal),
	)
	if err != nil || !found {
		return CognitionRuntimeSnapshotRecord{}, fmt.Errorf("%w: prepared snapshot journal is unavailable: %v", ErrCognitionConflict, err)
	}
	if journal.SnapshotSHA256 != snapshotSHA || journal.GraphVersion != graph.Version || journal.GraphSHA256 != graph.Graph.SHA256 {
		return CognitionRuntimeSnapshotRecord{}, fmt.Errorf("%w: prepared snapshot no longer binds the current graph", ErrCognitionConflict)
	}
	record, found, err := loadCognitionSnapshotReplayTx(
		ctx, tx, authority, episode, graph, obligationID, uint64(callOrdinal), journal.Budget,
	)
	if err != nil || !found {
		return CognitionRuntimeSnapshotRecord{}, fmt.Errorf("%w: restore prepared snapshot: %v", ErrCognitionConflict, err)
	}
	return record, nil
}

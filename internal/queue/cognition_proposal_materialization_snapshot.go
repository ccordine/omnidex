package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func loadCognitionProposalMaterializationSnapshotTx(
	ctx context.Context,
	tx pgx.Tx,
	episode CognitionEpisode,
	snapshotSHA256 string,
) (cognition.RuntimeSnapshot, uint64, error) {
	var (
		callOrdinal, revision, graphVersion, actorAttempt int64
		revisionSHA, obligationID, graphSHA               string
		projectionID, workingSetID, workerID              string
		jobID, generation, stepID                         int64
		budgetRaw, evidenceRaw                            []byte
	)
	if err := tx.QueryRow(ctx, `
		SELECT call_ordinal,expected_revision,expected_revision_sha256,obligation_node_id,
		       graph_version,graph_sha256,projection_id,working_set_id,job_id,generation,
		       step_id,actor_attempt,actor_worker_id,runtime_budget_json,evidence_refs_json
		FROM cognition_runtime_snapshots
		WHERE episode_id=$1 AND snapshot_sha256=$2
	`, episode.EpisodeID, snapshotSHA256).Scan(
		&callOrdinal, &revision, &revisionSHA, &obligationID, &graphVersion, &graphSHA,
		&projectionID, &workingSetID, &jobID, &generation, &stepID, &actorAttempt,
		&workerID, &budgetRaw, &evidenceRaw,
	); err != nil {
		return cognition.RuntimeSnapshot{}, 0, fmt.Errorf("load proposal materialization snapshot: %w", err)
	}
	if callOrdinal <= 0 || revision <= 0 || graphVersion <= 0 || actorAttempt <= 0 ||
		jobID != episode.Authority.JobID || generation != episode.Authority.Generation ||
		stepID != episode.Authority.StepID {
		return cognition.RuntimeSnapshot{}, 0, fmt.Errorf(
			"%w: proposal materialization snapshot authority changed", ErrCognitionConflict,
		)
	}
	graph, found, err := loadCognitionObligationGraphVersionTx(
		ctx, tx, episode.EpisodeID, uint64(graphVersion),
	)
	if err != nil || !found || graph.Graph.SHA256 != graphSHA {
		return cognition.RuntimeSnapshot{}, 0, fmt.Errorf(
			"%w: proposal materialization snapshot graph changed: %v", ErrCognitionConflict, err,
		)
	}
	current, found := cognitionTraceObligation(graph.Graph, cognition.ObligationID(obligationID))
	if !found || current.Status != cognition.ObligationActive {
		return cognition.RuntimeSnapshot{}, 0, fmt.Errorf(
			"%w: proposal materialization snapshot obligation changed", ErrCognitionConflict,
		)
	}
	projection, err := loadContextProjectionTx(ctx, tx, projectionID)
	attempt := model.StepAttemptAuthority{
		JobID: jobID, Generation: generation, StepID: stepID, Attempt: actorAttempt, WorkerID: workerID,
	}
	if err != nil || projection.Authority.Mode != ContextProjectionModeLive ||
		projection.Authority.StepAttemptAuthority != attempt ||
		string(projection.Projection.WorkingSetID) != workingSetID {
		return cognition.RuntimeSnapshot{}, 0, fmt.Errorf(
			"%w: proposal materialization projection changed: %v", ErrCognitionConflict, err,
		)
	}
	var budget cognition.RuntimeBudget
	var evidence []cognition.EvidenceRef
	if err := json.Unmarshal(budgetRaw, &budget); err != nil {
		return cognition.RuntimeSnapshot{}, 0, err
	}
	if err := json.Unmarshal(evidenceRaw, &evidence); err != nil || evidence == nil {
		return cognition.RuntimeSnapshot{}, 0, fmt.Errorf(
			"%w: proposal materialization evidence changed: %v", ErrCognitionConflict, err,
		)
	}
	revisionRef := cognition.WorldRevision{
		EpisodeID: episode.EpisodeID, Number: uint64(revision), SHA256: revisionSHA,
	}
	projectionRef := cognition.ContextProjectionRef{
		ID: cognition.ContextProjectionID(projectionID), SHA256: projection.Projection.RenderedSHA256,
		WorkingSetID:      cognition.WorkingSetID(workingSetID),
		WorkingSetVersion: projection.Projection.WorkingSetVersion,
		RendererVersion:   projection.Projection.RendererVersion,
	}
	snapshot, err := cognition.NewRuntimeSnapshot(
		episode.Goal, revisionRef, current, episode.ActionCatalog, cognitionAttempt(attempt),
		projectionRef, budget, evidence,
	)
	if err != nil || snapshot.SHA256() != snapshotSHA256 {
		return cognition.RuntimeSnapshot{}, 0, fmt.Errorf(
			"%w: proposal materialization snapshot identity changed: %v", ErrCognitionConflict, err,
		)
	}
	return snapshot, uint64(callOrdinal), nil
}

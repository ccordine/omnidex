package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func insertCognitionSnapshotJournalTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	prepared cognitionruntime.PreparedSnapshot,
	callOrdinal uint64,
) error {
	snapshot := prepared.Snapshot
	if err := prepared.ValidateFor(cognitionruntime.Binding{
		Episode: cognition.EpisodeRef{ID: snapshot.CurrentRevision().EpisodeID},
		Attempt: cognitionAttempt(authority),
	}); err != nil {
		return err
	}
	budgetJSON, budgetSHA, err := cognitionJSON(snapshot.Budget())
	if err != nil {
		return err
	}
	refs := snapshot.EvidenceRefs()
	if refs == nil {
		refs = []cognition.EvidenceRef{}
	}
	refsJSON, refsSHA, err := cognitionJSON(refs)
	if err != nil {
		return err
	}
	completionRefs := append([]cognition.EvidenceRef{}, prepared.CompletionEvidenceRefs...)
	completionRefsJSON, completionRefsSHA, err := cognitionJSON(completionRefs)
	if err != nil {
		return err
	}
	projection := snapshot.ContextProjection()
	_, err = tx.Exec(ctx, `
		INSERT INTO cognition_runtime_snapshots (
			snapshot_sha256,preparation_id,episode_id,job_id,generation,step_id,
			actor_attempt,actor_worker_id,call_ordinal,expected_revision,
			expected_revision_sha256,obligation_node_id,graph_version,graph_sha256,
			projection_id,working_set_id,runtime_budget_json,runtime_budget_sha256,
			evidence_refs_json,evidence_refs_sha256,completion_evidence_refs_json,
			completion_evidence_refs_sha256,environment_terminal,public_outcome
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)
	`, snapshot.SHA256(), "cognition_snapshot_"+snapshot.SHA256(), snapshot.CurrentRevision().EpisodeID,
		authority.JobID, authority.Generation, authority.StepID, authority.Attempt, authority.WorkerID,
		int64(callOrdinal), int64(snapshot.CurrentRevision().Number), snapshot.CurrentRevision().SHA256,
		snapshot.CurrentObligation().ID, int64(prepared.GraphVersion), prepared.ObligationGraph.SHA256,
		projection.ID, projection.WorkingSetID, string(budgetJSON), budgetSHA, string(refsJSON), refsSHA,
		string(completionRefsJSON), completionRefsSHA, prepared.EnvironmentTerminal, prepared.PublicOutcome)
	if err != nil {
		return fmt.Errorf("insert cognition runtime snapshot: %w", err)
	}
	return nil
}

func loadCognitionSnapshotReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	episode CognitionEpisode,
	graph CognitionObligationGraphRecord,
	obligationID cognition.ObligationID,
	callOrdinal uint64,
	budget cognition.RuntimeBudget,
) (CognitionRuntimeSnapshotRecord, bool, error) {
	journal, found, err := loadCognitionSnapshotJournalTx(
		ctx, tx, authority, episode, obligationID, callOrdinal,
	)
	if err != nil || !found {
		return CognitionRuntimeSnapshotRecord{}, found, err
	}
	if journal.GraphVersion != graph.Version || journal.GraphSHA256 != graph.Graph.SHA256 || journal.Budget != budget {
		return CognitionRuntimeSnapshotRecord{}, false, fmt.Errorf("%w: prepared snapshot replay authority changed", ErrCognitionConflict)
	}
	projection, err := loadContextProjectionTx(ctx, tx, journal.ProjectionID)
	if err != nil {
		return CognitionRuntimeSnapshotRecord{}, false, err
	}
	if projection.Authority.Mode != ContextProjectionModeLive || projection.Authority.StepAttemptAuthority != authority ||
		string(projection.Projection.WorkingSetID) != journal.WorkingSetID {
		return CognitionRuntimeSnapshotRecord{}, false, fmt.Errorf("%w: prepared live projection authority changed", ErrCognitionConflict)
	}
	current, err := oneActiveCognitionObligation(graph.Graph)
	if err != nil {
		return CognitionRuntimeSnapshotRecord{}, false, err
	}
	ref := cognition.ContextProjectionRef{
		ID: cognition.ContextProjectionID(projection.Projection.ID), SHA256: projection.Projection.RenderedSHA256,
		WorkingSetID:      cognition.WorkingSetID(projection.Projection.WorkingSetID),
		WorkingSetVersion: projection.Projection.WorkingSetVersion,
		RendererVersion:   projection.Projection.RendererVersion,
	}
	snapshot, err := cognition.NewRuntimeSnapshot(
		episode.Goal, episode.CurrentRevision, current, episode.ActionCatalog,
		cognitionAttempt(authority), ref, journal.Budget, journal.EvidenceRefs,
	)
	if err != nil {
		return CognitionRuntimeSnapshotRecord{}, false, err
	}
	if snapshot.SHA256() != journal.SnapshotSHA256 {
		return CognitionRuntimeSnapshotRecord{}, false, fmt.Errorf("%w: prepared snapshot hash changed", ErrCognitionConflict)
	}
	prepared := cognitionruntime.PreparedSnapshot{
		Snapshot: snapshot, ObligationGraph: graph.Graph.Clone(), GraphVersion: graph.Version,
		CompletionEvidenceRefs: append([]cognition.EvidenceRef{}, journal.CompletionEvidenceRefs...),
		EnvironmentTerminal:    journal.EnvironmentTerminal, PublicOutcome: journal.PublicOutcome,
	}
	if err := prepared.ValidateFor(cognitionruntime.Binding{
		Episode: cognition.EpisodeRef{ID: episode.EpisodeID}, Attempt: cognitionAttempt(authority),
	}); err != nil {
		return CognitionRuntimeSnapshotRecord{}, false, err
	}
	return CognitionRuntimeSnapshotRecord{Prepared: prepared, CallOrdinal: callOrdinal}, true, nil
}

func loadCognitionSnapshotJournalTx(
	ctx context.Context, tx pgx.Tx, authority model.StepAttemptAuthority, episode CognitionEpisode,
	obligationID cognition.ObligationID, callOrdinal uint64,
) (cognitionSnapshotJournal, bool, error) {
	var value cognitionSnapshotJournal
	var revision, graphVersion int64
	var budgetJSON, refsJSON, completionRefsJSON []byte
	err := tx.QueryRow(ctx, `
		SELECT snapshot_sha256,preparation_id,expected_revision,expected_revision_sha256,
		       graph_version,graph_sha256,projection_id,working_set_id,runtime_budget_json,
		       evidence_refs_json,completion_evidence_refs_json,environment_terminal,public_outcome
		FROM cognition_runtime_snapshots
		WHERE episode_id=$1 AND job_id=$2 AND generation=$3 AND step_id=$4
		  AND actor_attempt=$5 AND actor_worker_id=$6 AND call_ordinal=$7
		  AND expected_revision=$8 AND obligation_node_id=$9
	`, episode.EpisodeID, authority.JobID, authority.Generation, authority.StepID,
		authority.Attempt, authority.WorkerID, int64(callOrdinal), int64(episode.CurrentRevision.Number), obligationID).
		Scan(&value.SnapshotSHA256, &value.PreparationID, &revision, &value.Revision.SHA256,
			&graphVersion, &value.GraphSHA256, &value.ProjectionID, &value.WorkingSetID,
			&budgetJSON, &refsJSON, &completionRefsJSON, &value.EnvironmentTerminal, &value.PublicOutcome)
	if errors.Is(err, pgx.ErrNoRows) {
		return cognitionSnapshotJournal{}, false, nil
	}
	if err != nil {
		return cognitionSnapshotJournal{}, false, err
	}
	if revision <= 0 || graphVersion <= 0 {
		return cognitionSnapshotJournal{}, false, fmt.Errorf("%w: persisted cognition snapshot has invalid counters", ErrCognitionConflict)
	}
	value.CallOrdinal, value.ObligationID = callOrdinal, obligationID
	value.Revision = cognition.WorldRevision{EpisodeID: episode.EpisodeID, Number: uint64(revision), SHA256: value.Revision.SHA256}
	value.GraphVersion = uint64(graphVersion)
	if err := json.Unmarshal(budgetJSON, &value.Budget); err != nil {
		return cognitionSnapshotJournal{}, false, err
	}
	if err := json.Unmarshal(refsJSON, &value.EvidenceRefs); err != nil {
		return cognitionSnapshotJournal{}, false, err
	}
	if err := json.Unmarshal(completionRefsJSON, &value.CompletionEvidenceRefs); err != nil {
		return cognitionSnapshotJournal{}, false, err
	}
	if err := value.Budget.Validate(); err != nil {
		return cognitionSnapshotJournal{}, false, err
	}
	for _, ref := range value.EvidenceRefs {
		if err := ref.Validate(); err != nil {
			return cognitionSnapshotJournal{}, false, err
		}
	}
	for _, ref := range value.CompletionEvidenceRefs {
		if err := ref.Validate(); err != nil {
			return cognitionSnapshotJournal{}, false, err
		}
	}
	if value.EvidenceRefs == nil || value.CompletionEvidenceRefs == nil {
		return cognitionSnapshotJournal{}, false, fmt.Errorf(
			"%w: persisted model and completion evidence must be explicit arrays", ErrCognitionConflict,
		)
	}
	if value.Revision != episode.CurrentRevision || value.PreparationID != "cognition_snapshot_"+value.SnapshotSHA256 {
		return cognitionSnapshotJournal{}, false, fmt.Errorf("%w: persisted cognition snapshot is invalid", ErrCognitionConflict)
	}
	return value, true, nil
}

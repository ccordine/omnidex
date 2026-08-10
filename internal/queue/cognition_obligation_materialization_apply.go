package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func applyCognitionObligationMaterializationTx(
	ctx context.Context,
	tx pgx.Tx,
	header taskLedgerHeader,
	episode CognitionEpisode,
	record CognitionActionRecord,
	transition cognition.Transition,
	authority model.StepAttemptAuthority,
) (taskLedgerHeader, error) {
	link, found, err := loadCognitionActionGraphMaterializationTx(
		ctx, tx, record,
	)
	if err != nil || !found {
		return header, err
	}
	proof, err := cognitionTransitionProof(transition)
	if err != nil {
		return header, err
	}
	if transition.Terminal {
		return persistCognitionProposalDispositionTx(
			ctx, tx, header, episode, record, link,
			cognitionProposalRejectedTerminal, proof, authority,
		)
	}
	if link.Kind == cognition.ProposalPlanRevision {
		return applyCognitionPlanRevisionTx(
			ctx, tx, header, episode, record, transition, authority, link,
		)
	}
	if link.Kind != cognition.ProposalObligation || link.Obligation == nil {
		return header, fmt.Errorf("%w: graph materialization is not registered", ErrCognitionConflict)
	}
	materialization, graphVersion := *link.Obligation, link.Graph
	if err := requireCognitionEvidenceRefsTx(
		ctx, tx, episode.EpisodeID, transition.Current, materialization.Spec.SupportingRefs,
	); err != nil {
		return header, err
	}
	graph, found, err := loadCurrentCognitionObligationGraphTx(ctx, tx, episode.EpisodeID, true)
	if err != nil || !found {
		return header, fmt.Errorf("load materialization graph: %w", err)
	}
	if graph.Version != graphVersion || graph.Graph.SHA256 != materialization.ExpectedGraphSHA256 {
		return header, fmt.Errorf("%w: obligation materialization graph is stale", ErrCognitionConflict)
	}
	after, err := materialization.Apply(graph.Graph)
	if err != nil {
		return header, err
	}
	header, err = persistCognitionMaterializationTaskDiffTx(
		ctx, tx, header, episode, materialization, after,
	)
	if err != nil {
		return header, err
	}
	descriptor := cognitionObligationDescriptor{
		ID: materialization.ID, SHA256: materialization.SHA256,
		Kind: CognitionObligationMaterialize,
	}
	appended, err := insertCognitionObligationGraphTx(
		ctx, tx, episode.EpisodeID, graph.Version+1, descriptor, after, authority,
	)
	if err != nil {
		return header, err
	}
	if appended.Graph.SHA256 != materialization.ResultGraphSHA256 {
		return header, fmt.Errorf("%w: materialized graph hash changed", ErrCognitionConflict)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO cognition_obligation_materialization_applications (
			materialization_id,episode_id,action_id,input_graph_version,output_graph_version,
			transition_revision,result_graph_sha256,actor_attempt,actor_worker_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, materialization.ID, episode.EpisodeID, record.Action.ID, int64(graph.Version),
		int64(appended.Version), int64(transition.Current.Number), appended.Graph.SHA256,
		authority.Attempt, authority.WorkerID)
	if err != nil {
		return header, fmt.Errorf("record obligation materialization application: %w", err)
	}
	return persistCognitionProposalDispositionTx(
		ctx, tx, header, episode, record, link,
		cognitionProposalAcceptedMaterialization, proof, authority,
	)
}

type cognitionActionMaterialization struct {
	Materialization  cognition.ObligationMaterialization
	GraphVersion     uint64
	CandidateEntryID taskstate.EntryID
}

func loadCognitionActionMaterializationTx(
	ctx context.Context,
	tx pgx.Tx,
	record CognitionActionRecord,
) (cognitionActionMaterialization, bool, error) {
	var raw []byte
	var rawSHA string
	var graphVersion int64
	var candidateEntryID taskstate.EntryID
	err := tx.QueryRow(ctx, `
		SELECT descriptor_json,descriptor_json_sha256,expected_graph_version,candidate_entry_id
		FROM cognition_obligation_materializations WHERE reconciliation_id=$1
	`, record.ReconciliationID).Scan(&raw, &rawSHA, &graphVersion, &candidateEntryID)
	if errors.Is(err, pgx.ErrNoRows) {
		return cognitionActionMaterialization{}, false, nil
	}
	if err != nil {
		return cognitionActionMaterialization{}, false, err
	}
	if graphVersion <= 0 || candidateEntryID == "" || cognitionPayloadSHA(raw) != rawSHA {
		return cognitionActionMaterialization{}, false,
			fmt.Errorf("%w: obligation materialization persistence changed", ErrCognitionConflict)
	}
	var materialization cognition.ObligationMaterialization
	if err := json.Unmarshal(raw, &materialization); err != nil || materialization.Validate() != nil {
		return cognitionActionMaterialization{}, false,
			fmt.Errorf("%w: obligation materialization is invalid", ErrCognitionConflict)
	}
	wantRaw, _, err := cognitionJSON(materialization)
	if err != nil || !bytes.Equal(wantRaw, raw) {
		return cognitionActionMaterialization{}, false,
			fmt.Errorf("%w: obligation materialization is not canonical", ErrCognitionConflict)
	}
	decisionSHA, err := cognitionruntime.DecisionSHA256(record.Decision)
	if err != nil || materialization.EpisodeID != record.EpisodeID ||
		materialization.SourceSnapshotSHA256 != record.SnapshotSHA256 ||
		materialization.SourceDecisionSHA256 != decisionSHA ||
		materialization.ActiveObligationID != record.ObligationID {
		return cognitionActionMaterialization{}, false,
			fmt.Errorf("%w: obligation materialization differs from its action", ErrCognitionConflict)
	}
	return cognitionActionMaterialization{
		Materialization: materialization.Clone(), GraphVersion: uint64(graphVersion),
		CandidateEntryID: candidateEntryID,
	}, true, nil
}

func materializationTaskMetadata(
	episode CognitionEpisode,
	materialization cognition.ObligationMaterialization,
) (taskstate.JSONObject, error) {
	raw, _, err := cognitionJSON(struct {
		Schema            string              `json:"schema"`
		EpisodeID         cognition.EpisodeID `json:"episode_id"`
		JobGeneration     int64               `json:"job_generation"`
		PlanGeneration    uint64              `json:"plan_generation"`
		MaterializationID string              `json:"materialization_id"`
	}{cognitionQueueIdentitySchemaV1, episode.EpisodeID, episode.Authority.Generation,
		materialization.Generation, materialization.ID})
	if err != nil {
		return taskstate.JSONObject{}, err
	}
	return taskstate.NewJSONObject(raw)
}

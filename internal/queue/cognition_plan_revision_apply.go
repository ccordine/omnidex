package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func applyCognitionPlanRevisionTx(
	ctx context.Context,
	tx pgx.Tx,
	header taskLedgerHeader,
	episode CognitionEpisode,
	record CognitionActionRecord,
	transition cognition.Transition,
	authority model.StepAttemptAuthority,
	link cognitionActionGraphMaterialization,
) (taskLedgerHeader, error) {
	if link.Kind != cognition.ProposalPlanRevision || link.Revision == nil || link.Obligation != nil {
		return header, fmt.Errorf("%w: plan revision union is invalid", ErrCognitionConflict)
	}
	value := link.Revision.Clone()
	refs := append([]cognition.EvidenceRef{}, value.Root.SupportingRefs...)
	refs = append(refs, value.Next.SupportingRefs...)
	if err := requireCognitionEvidenceRefsTx(
		ctx, tx, episode.EpisodeID, transition.Current, refs,
	); err != nil {
		return header, err
	}
	graph, found, err := loadCurrentCognitionObligationGraphTx(ctx, tx, episode.EpisodeID, true)
	if err != nil || !found {
		return header, fmt.Errorf("load plan revision graph: %w", err)
	}
	if graph.Version != link.Graph || graph.Graph.SHA256 != value.ExpectedGraphSHA256 {
		return header, fmt.Errorf("%w: cognition plan revision graph is stale", ErrCognitionConflict)
	}
	after, err := value.Apply(graph.Graph)
	if err != nil {
		return header, err
	}
	header, err = persistCognitionPlanRevisionTaskTx(
		ctx, tx, header, episode, value, graph.Graph, after,
	)
	if err != nil {
		return header, err
	}
	descriptor := cognitionObligationDescriptor{
		ID: value.ID, SHA256: value.SHA256, Kind: CognitionObligationPlanRevise,
	}
	appended, err := insertCognitionObligationGraphTx(
		ctx, tx, episode.EpisodeID, graph.Version+1, descriptor, after, authority,
	)
	if err != nil {
		return header, err
	}
	if appended.Graph.SHA256 != value.ResultGraphSHA256 {
		return header, fmt.Errorf("%w: plan revision graph hash changed", ErrCognitionConflict)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO cognition_plan_revision_applications (
			plan_revision_id,episode_id,action_id,input_graph_version,output_graph_version,
			transition_revision,result_graph_sha256,actor_attempt,actor_worker_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, value.ID, episode.EpisodeID, record.Action.ID, int64(graph.Version),
		int64(appended.Version), int64(transition.Current.Number), appended.Graph.SHA256,
		authority.Attempt, authority.WorkerID)
	if err != nil {
		return header, fmt.Errorf("record cognition plan revision application: %w", err)
	}
	proof, err := cognitionTransitionProof(transition)
	if err != nil {
		return header, err
	}
	return persistCognitionProposalDispositionTx(
		ctx, tx, header, episode, record, link,
		cognitionProposalAcceptedMaterialization, proof, authority,
	)
}

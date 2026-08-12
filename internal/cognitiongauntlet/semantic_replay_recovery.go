package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/queue"
)

func (state *semanticReplayState) mapDecisionRecovery(
	record queue.CognitionSealedTraceRecord,
	source cognitionreplay.SourceRecord,
) ([]semanticEventDraft, error) {
	var value queue.CognitionTraceAcceptedDecisionRecovery
	if err := decodeProductionPayload(record.Payload, &value, "semantic decision recovery"); err != nil ||
		value.Validate() != nil || value.Recovery.ID != record.ID ||
		record.CallOrdinal < 1 || (record.Phase != 35 && record.Phase != 42) ||
		record.Sequence != int64(value.Binding.Attempt.Attempt) ||
		value.Binding.Episode.ID != state.trace.Header.EpisodeID {
		return nil, fmt.Errorf("invalid accepted decision recovery: %v", err)
	}
	if _, duplicate := state.recoveries[value.Recovery.ID]; duplicate {
		return nil, fmt.Errorf("semantic accepted-decision recovery is duplicated")
	}
	snapshot, exists := state.snapshots[value.SnapshotSHA256]
	decision, accepted := state.decisions[value.Recovery.PolicyCallID]
	graph, graphExists := state.graphs[value.GraphVersion]
	attempt, attemptExists := state.attempts[value.Recovery.PolicyCallID]
	if !exists || !accepted || !graphExists || !attemptExists ||
		value.GraphVersion != state.activeGraphVersion {
		return nil, fmt.Errorf("semantic accepted-decision recovery lacks its source authority")
	}
	decisionSHA, digestErr := cognitionruntime.DecisionSHA256(decision)
	schema, schemaExists := snapshot.snapshot.ActionCatalog().Schema(decision.Action.Kind)
	if snapshot.callOrdinal != record.CallOrdinal || digestErr != nil ||
		decisionSHA != value.DecisionSHA256 || value.SourceActor != attempt.Actor ||
		value.ContextProjection != snapshot.snapshot.ContextProjection() ||
		value.ObligationID != snapshot.snapshot.CurrentObligation().ID ||
		value.GraphSHA256 != graph.SHA256 || !schemaExists || schema.Ref() != value.ActionSchema ||
		value.CreatedAt.Before(state.trace.Header.EpisodeStartedAt) ||
		value.CreatedAt.After(state.trace.Header.SealedAt) {
		return nil, fmt.Errorf("semantic accepted-decision recovery differs from durable authority")
	}
	state.recoveries[value.Recovery.ID] = value
	return []semanticEventDraft{sourceDraft(cognitionreplay.EventEpisodeRestarted, source)}, nil
}

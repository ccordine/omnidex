package cognitiongauntlet

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/queue"
)

func (state *semanticReplayState) mapEpisodeProgress(
	record queue.CognitionSealedTraceRecord,
	source cognitionreplay.SourceRecord,
) ([]semanticEventDraft, error) {
	var value cognitionruntime.EpisodeProgress
	if err := decodeProductionPayload(record.Payload, &value, "semantic episode progress"); err != nil ||
		validateSemanticProgress(value, state.trace.Header.EpisodeID) != nil ||
		record.CallOrdinal < 1 || record.Phase != 71 || record.Sequence < 2 {
		return nil, fmt.Errorf("invalid semantic episode progress: %v", err)
	}
	command, exists := state.progressCommands[record.ID]
	if !exists || command.callOrdinal != record.CallOrdinal || command.sequence != record.Sequence {
		return nil, fmt.Errorf("semantic episode progress lacks its exact command")
	}
	if _, duplicate := state.progressResults[record.ID]; duplicate {
		return nil, fmt.Errorf("semantic episode progress is duplicated")
	}
	snapshot, exists := state.snapshots[command.command.SnapshotSHA256]
	if !exists || snapshot.callOrdinal != record.CallOrdinal ||
		queue.VerifyCognitionRuntimeProgressCommandID(record.ID, command.command, value) != nil ||
		cognitionruntime.VerifyEpisodeProgress(snapshot.snapshot, command.command, value) != nil ||
		value.GraphVersion != uint64(record.Sequence) {
		return nil, fmt.Errorf("semantic episode progress changed its exact prepared authority")
	}
	state.progressResults[record.ID] = cloneSemanticProgress(value)
	revision := semanticReplayRevision(value.Revision)
	goalRef := "goal://" + string(value.Episode.ID)
	var drafts []semanticEventDraft
	add := func(kind cognitionreplay.EventKind, status cognitionreplay.KnowledgeStatus) {
		drafts = append(drafts, semanticEventDraft{
			Kind: kind, Revision: revision, Payload: source.Payload,
			Knowledge: knowledgeChange(
				cognitionreplay.KnowledgeGoal, goalRef, status, cognitionreplay.AuthorityCode,
			),
		})
	}
	switch value.State {
	case cognitionruntime.ProgressActive:
		drafts = append(drafts, semanticEventDraft{
			Kind: cognitionreplay.EventObligationChanged, Revision: revision,
			Payload: source.Payload,
		})
	case cognitionruntime.ProgressCompleted:
		if state.terminalProgress != nil || state.cancellation != nil ||
			state.trace.Header.Seal.Outcome != queue.CognitionEpisodeCompleted {
			return nil, fmt.Errorf("completed semantic progress conflicts with terminal authority")
		}
		add(cognitionreplay.EventGoalSatisfied, cognitionreplay.KnowledgeSatisfied)
		drafts = append(drafts, sourceDraft(cognitionreplay.EventEpisodeSealed, source))
		terminal := cloneSemanticProgress(value)
		state.terminalProgress = &terminal
		state.terminalProgressCommandID = record.ID
		state.terminal = true
	case cognitionruntime.ProgressFailed:
		if state.terminalProgress != nil || state.cancellation != nil ||
			state.trace.Header.Seal.Outcome != queue.CognitionEpisodeFailed {
			return nil, fmt.Errorf("failed semantic progress conflicts with terminal authority")
		}
		add(cognitionreplay.EventGoalFailed, cognitionreplay.KnowledgeFailed)
		drafts = append(drafts,
			sourceKnowledgeDraft(
				cognitionreplay.EventFailureRecorded, source,
				cognitionreplay.KnowledgeFailure, cognitionreplay.KnowledgeFailed,
				cognitionreplay.AuthorityCode,
			),
			sourceDraft(cognitionreplay.EventEpisodeSealed, source),
		)
		terminal := cloneSemanticProgress(value)
		state.terminalProgress = &terminal
		state.terminalProgressCommandID = record.ID
		state.terminal = true
	default:
		return nil, fmt.Errorf("unregistered episode progress state %q", value.State)
	}
	return drafts, nil
}

func (state *semanticReplayState) mapEpisodeProgressCommand(
	record queue.CognitionSealedTraceRecord,
	source cognitionreplay.SourceRecord,
) ([]semanticEventDraft, error) {
	var value cognitionruntime.CompletionCommand
	if err := decodeProductionPayload(record.Payload, &value, "semantic episode progress command"); err != nil ||
		value.Binding.Validate() != nil || value.Binding.Episode.ID != state.trace.Header.EpisodeID ||
		value.GraphVersion == 0 || value.ObligationGraph.Validate() != nil ||
		value.Result.Validate() != nil || !validDigest(value.SnapshotSHA256) ||
		record.CallOrdinal < 1 || record.Phase != 70 || record.Sequence < 2 ||
		uint64(record.Sequence) != value.GraphVersion+1 {
		return nil, fmt.Errorf("invalid semantic episode progress command: %v", err)
	}
	if _, duplicate := state.progressCommands[record.ID]; duplicate {
		return nil, fmt.Errorf("semantic episode progress command is duplicated")
	}
	snapshot, exists := state.snapshots[value.SnapshotSHA256]
	graph, graphExists := state.graphs[value.GraphVersion]
	if !exists || snapshot.callOrdinal != record.CallOrdinal ||
		value.Binding.Attempt != snapshot.snapshot.Attempt() || !graphExists ||
		value.GraphVersion != state.activeGraphVersion ||
		!reflect.DeepEqual(graph, value.ObligationGraph) {
		return nil, fmt.Errorf("semantic episode progress command lacks its snapshot or input graph")
	}
	if err := state.consumeSnapshot(
		value.SnapshotSHA256, "episode-progress://"+record.ID,
	); err != nil {
		return nil, err
	}
	state.progressCommands[record.ID] = semanticProgressCommandRecord{
		command: value, callOrdinal: record.CallOrdinal, sequence: record.Sequence,
	}
	return []semanticEventDraft{sourceDraft(
		cognitionreplay.EventObligationChanged, source,
	)}, nil
}

func (state *semanticReplayState) mapCancellation(
	record queue.CognitionSealedTraceRecord,
	source cognitionreplay.SourceRecord,
) ([]semanticEventDraft, error) {
	var value cognitionruntime.CancellationEvidence
	if err := decodeProductionPayload(record.Payload, &value, "semantic cancellation"); err != nil ||
		value.Validate() != nil || value.ID != record.ID || record.CallOrdinal != 0 ||
		record.Phase != 80 || record.Sequence != 0 || state.cancellation != nil ||
		state.terminalProgress != nil || state.trace.Header.Seal.Outcome != queue.CognitionEpisodeCanceled {
		return nil, fmt.Errorf("invalid cancellation evidence: %v", err)
	}
	copy := value
	state.cancellation = &copy
	revision := semanticReplayRevision(state.trace.Header.Seal.FinalRevision)
	state.terminal = true
	return []semanticEventDraft{
		{Kind: cognitionreplay.EventFailureRecorded, Revision: revision, Payload: source.Payload,
			Knowledge: knowledgeChange(
				cognitionreplay.KnowledgeFailure, "failure://"+value.ID,
				cognitionreplay.KnowledgeFailed, cognitionreplay.AuthorityCode,
			)},
		{Kind: cognitionreplay.EventEpisodeCanceled, Revision: revision, Payload: source.Payload},
		{Kind: cognitionreplay.EventEpisodeSealed, Revision: revision, Payload: source.Payload},
	}, nil
}

func (state *semanticReplayState) mapObligationGraph(
	record queue.CognitionSealedTraceRecord,
	source cognitionreplay.SourceRecord,
) ([]semanticEventDraft, error) {
	var graph cognition.ObligationGraphSnapshot
	if err := decodeProductionPayload(record.Payload, &graph, "semantic obligation graph"); err != nil ||
		graph.Validate() != nil || record.Sequence < 1 ||
		(record.Sequence == 1 && (record.CallOrdinal != 0 || record.Phase != 20)) ||
		(record.Sequence > 1 && record.Phase != 72) ||
		(graph.SHA256 != state.trace.Header.GraphSHA256 &&
			uint64(record.Sequence) == state.trace.Header.GraphVersion) {
		return nil, fmt.Errorf("invalid semantic obligation graph: %v", err)
	}
	version := uint64(record.Sequence)
	if version == 1 {
		if err := state.verifyInitialObligationGraph(record.ID, graph); err != nil {
			return nil, err
		}
	}
	if _, duplicate := state.graphs[version]; duplicate {
		return nil, fmt.Errorf("semantic obligation graph version is duplicated")
	}
	state.graphs[version] = graph.Clone()
	state.graphRecordIDs[version] = record.ID
	if version == 1 {
		state.classifiedGraphs[version] = "initial"
	}
	progress, progressExists := state.progressResults[record.ID]
	if progressExists {
		if record.CallOrdinal < 1 || progress.GraphVersion != version ||
			!reflect.DeepEqual(progress.ObligationGraph, graph) {
			return nil, fmt.Errorf("semantic obligation graph differs from its progress")
		}
		state.classifiedGraphs[version] = "progress"
	}
	if version > 1 && record.CallOrdinal == 0 {
		if err := state.deferSource(source); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if version > 1 && !progressExists {
		return nil, fmt.Errorf("semantic obligation graph lacks its exact progress result")
	}
	return state.activateObligationGraph(version, source)
}

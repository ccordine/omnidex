package cognitiongauntlet

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/workingset"
)

func (state *semanticReplayState) mapWorkingSetSnapshot(
	record queue.CognitionSealedTraceRecord,
	source cognitionreplay.SourceRecord,
) ([]semanticEventDraft, error) {
	var value queue.CognitionTraceWorkingSetSnapshot
	if err := decodeProductionPayload(record.Payload, &value, "semantic Working Set snapshot"); err != nil {
		return nil, err
	}
	if value.Schema != queue.CognitionTraceWorkingSetSnapshotSchemaV1 ||
		value.CapturedAt.Before(state.trace.Header.EpisodeStartedAt) ||
		value.CapturedAt.After(state.trace.Header.SealedAt) ||
		workingset.ValidateSnapshot(value.Snapshot) != nil {
		return nil, fmt.Errorf("invalid semantic Working Set snapshot")
	}
	draft := sourceDraft(cognitionreplay.EventWorkingSetSnapshot, source)
	switch value.Point {
	case "episode_start":
		if state.verifyWorkingSetTuple(
			record, queue.CognitionWorkingSetTraceEpisodeStart, value.Snapshot.Version,
		) != nil || state.workingSet != nil ||
			record.ID != string(value.Snapshot.ID)+":episode-start" {
			return nil, fmt.Errorf("semantic Working Set start snapshot is duplicated or changed")
		}
		set, err := workingset.Restore(value.Snapshot)
		if err != nil {
			return nil, fmt.Errorf("restore semantic Working Set start: %w", err)
		}
		state.workingSet = set
		for _, item := range value.Snapshot.Items {
			draft.KnowledgeChanges = append(draft.KnowledgeChanges, workingSetKnowledge(item))
		}
	case "terminal":
		if state.verifyWorkingSetTuple(
			record, queue.CognitionWorkingSetTraceTerminal, value.Snapshot.Version,
		) != nil || state.workingSet == nil || state.workingSetTerminal ||
			record.ID != string(value.Snapshot.ID)+":terminal" ||
			value.Snapshot.Version != state.trace.Header.WorkingSetVersion ||
			!reflect.DeepEqual(state.workingSet.Snapshot(), value.Snapshot) {
			return nil, fmt.Errorf("semantic Working Set terminal snapshot differs from exact replay")
		}
		state.workingSetTerminal = true
	default:
		return nil, fmt.Errorf("semantic Working Set snapshot point %q is not registered", value.Point)
	}
	return []semanticEventDraft{draft}, nil
}

func workingSetKnowledge(item workingset.Item) *semanticKnowledgeChange {
	status := cognitionreplay.KnowledgeActive
	switch item.State {
	case workingset.ItemReleased:
		status = cognitionreplay.KnowledgeReleased
	case workingset.ItemInvalidated:
		status = cognitionreplay.KnowledgeStale
	}
	return knowledgeChange(
		cognitionreplay.KnowledgeWorkingSet, "working-set-item://"+string(item.ID),
		status, cognitionreplay.AuthorityCode,
	)
}

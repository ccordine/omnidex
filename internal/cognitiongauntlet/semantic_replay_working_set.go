package cognitiongauntlet

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/workingset"
)

func (state *semanticReplayState) mapWorkingSetEvent(
	record queue.CognitionSealedTraceRecord,
	source cognitionreplay.SourceRecord,
) ([]semanticEventDraft, error) {
	var value queue.CognitionTraceWorkingSetEvent
	if err := decodeProductionPayload(record.Payload, &value, "semantic Working Set event"); err != nil {
		return nil, err
	}
	if state.workingSet == nil || state.workingSetTerminal ||
		value.Schema != queue.CognitionTraceWorkingSetEventSchemaV1 ||
		value.CreatedAt.Before(state.trace.Header.EpisodeStartedAt) ||
		value.CreatedAt.After(state.trace.Header.SealedAt) ||
		workingset.ValidateEvent(value.Event) != nil || value.Event.SetID != state.workingSet.ID() ||
		value.Event.Version != state.workingSet.Version()+1 ||
		record.ID != fmt.Sprintf("%s:event:%d", value.Event.SetID, value.Event.Version) {
		return nil, fmt.Errorf("invalid semantic Working Set event")
	}
	if err := state.verifyWorkingSetTuple(
		record, queue.CognitionWorkingSetTraceEvent, value.Event.Version,
	); err != nil {
		return nil, fmt.Errorf("invalid semantic Working Set event tuple: %w", err)
	}
	before := workingSetItems(state.workingSet.Items())
	command, err := workingset.DecodeCommand(value.Event.CommandKind, value.Event.Command)
	if err != nil {
		return nil, err
	}
	actual, err := state.workingSet.Apply(command)
	if err != nil || !reflect.DeepEqual(actual, value.Event) {
		return nil, fmt.Errorf("semantic Working Set event replay diverged: %v", err)
	}
	after := workingSetItems(state.workingSet.Items())
	return semanticWorkingSetDrafts(source, command, before, after)
}

func semanticWorkingSetDrafts(
	source cognitionreplay.SourceRecord,
	command workingset.Command,
	before, after map[workingset.ItemID]workingset.Item,
) ([]semanticEventDraft, error) {
	changed := changedWorkingSetItems(before, after)
	drafts := make([]semanticEventDraft, 0, len(changed)+1)
	appendItem := func(kind cognitionreplay.EventKind, id workingset.ItemID) {
		draft := sourceDraft(kind, source)
		draft.Knowledge = workingSetKnowledge(after[id])
		drafts = append(drafts, draft)
		delete(changed, id)
	}
	switch value := command.(type) {
	case *workingset.AcquireCommand:
		if _, existed := before[value.Request.ID]; existed || after[value.Request.ID].State != workingset.ItemResident {
			return nil, fmt.Errorf("semantic Working Set acquire did not attach its exact item")
		}
		appendItem(cognitionreplay.EventWorkingSetAttached, value.Request.ID)
	case *workingset.ReacquireCommand:
		if before[value.Request.ItemID].State != workingset.ItemReleased ||
			after[value.Request.ItemID].State != workingset.ItemResident {
			return nil, fmt.Errorf("semantic Working Set reacquire did not restore its exact item")
		}
		appendItem(cognitionreplay.EventWorkingSetReacquired, value.Request.ItemID)
	case *workingset.RetainCommand:
		appendItem(cognitionreplay.EventWorkingSetRetained, value.ItemID)
	case *workingset.ReleaseCommand:
		kind := cognitionreplay.EventWorkingSetRetained
		if after[value.ItemID].State == workingset.ItemReleased {
			kind = cognitionreplay.EventWorkingSetReleased
		}
		appendItem(kind, value.ItemID)
	case *workingset.TouchCommand:
		ids := append([]workingset.ItemID(nil), value.ItemIDs...)
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for _, id := range ids {
			appendItem(cognitionreplay.EventWorkingSetTouched, id)
		}
	case *workingset.InvalidateStaleCommand:
		if after[value.ItemID].State != workingset.ItemInvalidated {
			return nil, fmt.Errorf("semantic Working Set invalidation did not stale its exact item")
		}
		appendItem(cognitionreplay.EventWorkingSetInvalidated, value.ItemID)
	case *workingset.CloseScopeCommand:
		drafts = append(drafts, sourceDraft(cognitionreplay.EventWorkingSetScopeClosed, source))
	default:
		return nil, fmt.Errorf("unregistered Working Set command")
	}
	for _, id := range sortedWorkingSetIDs(changed) {
		prior, existed := before[id]
		current, exists := after[id]
		kind := cognitionreplay.EventWorkingSetRetained
		if !existed || !exists || prior.State != workingset.ItemResident {
			return nil, fmt.Errorf("semantic Working Set command changed an unexpected item %q", id)
		}
		switch current.State {
		case workingset.ItemReleased:
			kind = cognitionreplay.EventWorkingSetReleased
		case workingset.ItemInvalidated:
			kind = cognitionreplay.EventWorkingSetInvalidated
		case workingset.ItemResident:
		default:
			return nil, fmt.Errorf("semantic Working Set item %q has an invalid result", id)
		}
		appendItem(kind, id)
	}
	if len(drafts) == 0 {
		return nil, fmt.Errorf("semantic Working Set command has no exact semantic event")
	}
	return drafts, nil
}

func workingSetItems(values []workingset.Item) map[workingset.ItemID]workingset.Item {
	result := make(map[workingset.ItemID]workingset.Item, len(values))
	for _, item := range values {
		result[item.ID] = item
	}
	return result
}

func changedWorkingSetItems(
	before, after map[workingset.ItemID]workingset.Item,
) map[workingset.ItemID]struct{} {
	result := make(map[workingset.ItemID]struct{})
	for id, prior := range before {
		if current, exists := after[id]; !exists || !reflect.DeepEqual(prior, current) {
			result[id] = struct{}{}
		}
	}
	for id, current := range after {
		if prior, exists := before[id]; !exists || !reflect.DeepEqual(prior, current) {
			result[id] = struct{}{}
		}
	}
	return result
}

func sortedWorkingSetIDs(values map[workingset.ItemID]struct{}) []workingset.ItemID {
	result := make([]workingset.ItemID, 0, len(values))
	for id := range values {
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

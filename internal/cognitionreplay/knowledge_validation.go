package cognitionreplay

import (
	"fmt"
	"reflect"
	"sort"
)

func prepareKnowledgeCheckpoints(values []KnowledgeCheckpoint) ([]KnowledgeCheckpoint, error) {
	result := cloneKnowledgeCheckpoints(values)
	previous := ""
	for index := range result {
		checkpoint := &result[index]
		if checkpoint.StateSHA256 != "" || checkpoint.DeltaSHA256 != "" ||
			checkpoint.CheckpointSHA256 != "" || checkpoint.PreviousSHA256 != "" {
			return nil, fmt.Errorf("unprepared knowledge checkpoint %d contains derived identity", index+1)
		}
		checkpoint.PreviousSHA256 = previous
		stateSHA, err := digestCanonical(checkpoint.State)
		if err != nil {
			return nil, err
		}
		checkpoint.StateSHA256 = stateSHA
		if checkpoint.Delta != nil {
			checkpoint.DeltaSHA256, err = digestCanonical(checkpoint.Delta)
			if err != nil {
				return nil, err
			}
		}
		checkpoint.CheckpointSHA256, err = checkpointDigest(*checkpoint)
		if err != nil {
			return nil, err
		}
		previous = checkpoint.CheckpointSHA256
	}
	return result, nil
}

func validateKnowledgeCheckpoints(values []KnowledgeCheckpoint, eventCount int) error {
	if len(values) < 2 || len(values) > maxCheckpoints {
		return fmt.Errorf("public replay requires bounded initial and final knowledge checkpoints")
	}
	for index, checkpoint := range values {
		if checkpoint.Sequence != uint64(index+1) || checkpoint.AfterEvent > uint64(eventCount) ||
			checkpoint.StateSHA256 == "" || checkpoint.CheckpointSHA256 == "" {
			return fmt.Errorf("knowledge checkpoint %d identity is invalid", index+1)
		}
		if err := validateKnowledgeState(checkpoint.State, checkpoint.AfterEvent); err != nil {
			return fmt.Errorf("knowledge checkpoint %d: %w", index+1, err)
		}
		stateSHA, err := digestCanonical(checkpoint.State)
		if err != nil || stateSHA != checkpoint.StateSHA256 {
			return fmt.Errorf("knowledge checkpoint %d state hash changed", index+1)
		}
		wantPrevious := ""
		if index > 0 {
			wantPrevious = values[index-1].CheckpointSHA256
		}
		if checkpoint.PreviousSHA256 != wantPrevious {
			return fmt.Errorf("knowledge checkpoint %d chain changed", index+1)
		}
		wantCheckpoint, err := checkpointDigest(checkpoint)
		if err != nil || wantCheckpoint != checkpoint.CheckpointSHA256 {
			return fmt.Errorf("knowledge checkpoint %d hash changed", index+1)
		}
		if index == 0 {
			if checkpoint.AfterEvent != 0 || checkpoint.Delta != nil || checkpoint.DeltaSHA256 != "" ||
				checkpoint.State.Revision != nil || len(checkpoint.State.Entries) != 0 {
				return fmt.Errorf("initial knowledge checkpoint is not an empty public state")
			}
			continue
		}
		previous := values[index-1]
		if checkpoint.Delta == nil || checkpoint.DeltaSHA256 == "" ||
			checkpoint.AfterEvent <= previous.AfterEvent ||
			checkpoint.AfterEvent-previous.AfterEvent > maxCheckpointInterval {
			return fmt.Errorf("knowledge checkpoint %d lacks one bounded forward delta", index+1)
		}
		if index < len(values)-1 &&
			checkpoint.AfterEvent-previous.AfterEvent != maxCheckpointInterval {
			return fmt.Errorf("knowledge checkpoint %d is not at the canonical interval", index+1)
		}
		if err := validateKnowledgeDelta(*checkpoint.Delta, previous.AfterEvent, checkpoint.AfterEvent); err != nil {
			return fmt.Errorf("knowledge checkpoint %d: %w", index+1, err)
		}
		deltaSHA, err := digestCanonical(checkpoint.Delta)
		if err != nil || deltaSHA != checkpoint.DeltaSHA256 {
			return fmt.Errorf("knowledge checkpoint %d delta hash changed", index+1)
		}
		applied, err := applyKnowledgeDelta(previous.State, *checkpoint.Delta)
		if err != nil || !reflect.DeepEqual(applied, checkpoint.State) {
			return fmt.Errorf("knowledge checkpoint %d does not equal its exact delta: %v", index+1, err)
		}
	}
	if values[len(values)-1].AfterEvent != uint64(eventCount) {
		return fmt.Errorf("final knowledge checkpoint does not cover every replay event")
	}
	return nil
}

func validateKnowledgeState(state KnowledgeState, afterEvent uint64) error {
	if state.Schema != KnowledgeStateSchemaV1 || state.Entries == nil {
		return fmt.Errorf("public knowledge state schema or entries are invalid")
	}
	if state.Revision != nil && state.Revision.Validate() != nil {
		return fmt.Errorf("public knowledge state revision is invalid")
	}
	for index, entry := range state.Entries {
		if err := validateKnowledgeEntry(entry, afterEvent); err != nil {
			return fmt.Errorf("knowledge entry %d: %w", index+1, err)
		}
		if index > 0 && !knowledgeEntryLess(state.Entries[index-1], entry) {
			return fmt.Errorf("public knowledge entries are duplicated or out of order")
		}
	}
	return nil
}

func validateKnowledgeEntry(entry KnowledgeEntry, afterEvent uint64) error {
	if !validKnowledgeKind(entry.Kind) || requireExact(entry.Ref, "knowledge reference") != nil ||
		!validKnowledgeStatus(entry.Status) || !validKnowledgeAuthority(entry.Authority) ||
		entry.Content.Validate() != nil || len(entry.SourceEvents) == 0 || len(entry.SourceEvents) > maxEvents {
		return fmt.Errorf("public knowledge entry authority is invalid")
	}
	for index, event := range entry.SourceEvents {
		if event == 0 || event > afterEvent || (index > 0 && event <= entry.SourceEvents[index-1]) {
			return fmt.Errorf("public knowledge source events are invalid")
		}
	}
	return nil
}

func validateKnowledgeDelta(delta KnowledgeDelta, previous, current uint64) error {
	if delta.Schema != KnowledgeDeltaSchemaV1 || delta.FromEvent != previous+1 ||
		delta.ThroughEvent != current || delta.Upserts == nil || delta.Releases == nil {
		return fmt.Errorf("public knowledge delta range or arrays are invalid")
	}
	if delta.SetRevision != nil && delta.SetRevision.Validate() != nil {
		return fmt.Errorf("public knowledge delta revision is invalid")
	}
	for index, entry := range delta.Upserts {
		if err := validateKnowledgeEntry(entry, current); err != nil ||
			latestEvent(entry.SourceEvents) < delta.FromEvent {
			return fmt.Errorf("public knowledge delta upsert %d is not interval-bound", index+1)
		}
		if index > 0 && !knowledgeEntryLess(delta.Upserts[index-1], entry) {
			return fmt.Errorf("public knowledge delta upserts are duplicated or out of order")
		}
	}
	for index, release := range delta.Releases {
		if !validKnowledgeKind(release.Kind) || requireExact(release.Ref, "released knowledge reference") != nil ||
			release.SourceEvent < delta.FromEvent || release.SourceEvent > current ||
			(index > 0 && !knowledgeReleaseLess(delta.Releases[index-1], release)) {
			return fmt.Errorf("public knowledge delta release %d is invalid", index+1)
		}
	}
	return nil
}

func applyKnowledgeDelta(state KnowledgeState, delta KnowledgeDelta) (KnowledgeState, error) {
	entries := make(map[string]KnowledgeEntry, len(state.Entries)+len(delta.Upserts))
	for _, entry := range state.Entries {
		entries[knowledgeKey(entry.Kind, entry.Ref)] = entry
	}
	for _, release := range delta.Releases {
		key := knowledgeKey(release.Kind, release.Ref)
		if _, exists := entries[key]; !exists {
			return KnowledgeState{}, fmt.Errorf("knowledge delta releases an absent entry")
		}
		delete(entries, key)
	}
	for _, entry := range delta.Upserts {
		entries[knowledgeKey(entry.Kind, entry.Ref)] = entry
	}
	result := KnowledgeState{Schema: KnowledgeStateSchemaV1, Revision: cloneRevision(state.Revision)}
	if delta.SetRevision != nil {
		result.Revision = cloneRevision(delta.SetRevision)
	}
	result.Entries = make([]KnowledgeEntry, 0, len(entries))
	for _, entry := range entries {
		result.Entries = append(result.Entries, cloneKnowledgeEntry(entry))
	}
	sort.Slice(result.Entries, func(left, right int) bool {
		return knowledgeEntryLess(result.Entries[left], result.Entries[right])
	})
	return result, nil
}

func checkpointDigest(value KnowledgeCheckpoint) (string, error) {
	value.CheckpointSHA256 = ""
	return digestCanonical(value)
}

func latestEvent(values []uint64) uint64 {
	if len(values) == 0 {
		return 0
	}
	return values[len(values)-1]
}

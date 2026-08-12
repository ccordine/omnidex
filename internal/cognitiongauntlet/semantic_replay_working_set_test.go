package cognitiongauntlet

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func TestSemanticReplayWorkingSetReplaysLifecycleAndPreservesProvenance(t *testing.T) {
	finalSet, events := diagnosticWorkingSet(t)
	start, err := workingset.New(finalSet.Owner(), finalSet.Budget())
	if err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, time.August, 12, 2, 0, 0, 0, time.UTC)
	state := semanticWorkingSetTestState(started, finalSet.Version())
	semanticAcceptWorkingSnapshot(t, state, 1, started, "episode_start", start.Snapshot())
	for index, event := range events {
		semanticAcceptWorkingEvent(t, state, index+2, started.Add(time.Duration(index+1)*time.Millisecond), event)
	}
	semanticAcceptWorkingSnapshot(t, state, len(events)+2, started.Add(time.Second), "terminal", finalSet.Snapshot())

	wantKinds := []cognitionreplay.EventKind{
		cognitionreplay.EventWorkingSetSnapshot,
		cognitionreplay.EventWorkingSetAttached,
		cognitionreplay.EventWorkingSetReleased,
		cognitionreplay.EventWorkingSetReacquired,
		cognitionreplay.EventWorkingSetSnapshot,
	}
	gotKinds := make([]cognitionreplay.EventKind, len(state.events))
	for index := range state.events {
		gotKinds[index] = state.events[index].Kind
	}
	if !reflect.DeepEqual(gotKinds, wantKinds) {
		t.Fatalf("Working Set semantic events=%v, want %v", gotKinds, wantKinds)
	}
	entry := state.entries["working_set\x00working-set-item://diagnostic-item"]
	if entry.Status != cognitionreplay.KnowledgeActive ||
		!reflect.DeepEqual(entry.SourceEvents, []uint64{2, 3, 4}) {
		t.Fatalf("reacquired Working Set knowledge=%+v", entry)
	}
	state.appendCheckpoint()
	final := state.checkpoints[len(state.checkpoints)-1]
	if len(final.Delta.Upserts) != 1 ||
		!reflect.DeepEqual(final.Delta.Upserts[0].SourceEvents, []uint64{2, 3, 4}) {
		t.Fatalf("Working Set checkpoint lost lifecycle provenance: %+v", final.Delta)
	}
}

func TestSemanticReplayWorkingSetDerivesEvictionAndRejectsFalseEndpoint(t *testing.T) {
	set, first, second := semanticEvictingWorkingSet(t)
	started := time.Date(2026, time.August, 12, 3, 0, 0, 0, time.UTC)
	start, err := workingset.New(set.Owner(), set.Budget())
	if err != nil {
		t.Fatal(err)
	}
	state := semanticWorkingSetTestState(started, set.Version())
	semanticAcceptWorkingSnapshot(t, state, 1, started, "episode_start", start.Snapshot())
	semanticAcceptWorkingEvent(t, state, 2, started.Add(time.Millisecond), first)
	semanticAcceptWorkingEvent(t, state, 3, started.Add(2*time.Millisecond), second)

	got := []cognitionreplay.EventKind{
		state.events[1].Kind, state.events[2].Kind, state.events[3].Kind,
	}
	want := []cognitionreplay.EventKind{
		cognitionreplay.EventWorkingSetAttached,
		cognitionreplay.EventWorkingSetAttached,
		cognitionreplay.EventWorkingSetReleased,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("evicting acquire events=%v, want %v", got, want)
	}
	forgedSet, err := workingset.Restore(start.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	command, err := workingset.DecodeCommand(first.CommandKind, first.Command)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := forgedSet.Apply(command); err != nil {
		t.Fatal(err)
	}
	forged := forgedSet.Snapshot()
	raw := semanticWorkingSnapshotPayload(t, "terminal", started.Add(time.Second), forged)
	if _, err := state.mapWorkingSetSnapshot(
		semanticWorkingSnapshotRecord(t, "terminal", started.Add(time.Second), forged),
		semanticWorkingSource(t, 4,
			semanticWorkingSnapshotRecord(t, "terminal", started.Add(time.Second), forged), raw),
	); err == nil {
		t.Fatal("semantic replay accepted a terminal Working Set snapshot outside exact replay")
	}
}

func TestSemanticReplayWorkingSetMapsEveryCommandFromActualStateDiff(t *testing.T) {
	set, start, events := semanticWorkingSetLifecycle(t)
	started := time.Date(2026, time.August, 12, 4, 0, 0, 0, time.UTC)
	state := semanticWorkingSetTestState(started, set.Version())
	semanticAcceptWorkingSnapshot(t, state, 1, started, "episode_start", start)
	for index, event := range events {
		semanticAcceptWorkingEvent(t, state, index+2,
			started.Add(time.Duration(index+1)*time.Millisecond), event)
	}
	semanticAcceptWorkingSnapshot(t, state, len(events)+2,
		started.Add(time.Second), "terminal", set.Snapshot())

	want := []cognitionreplay.EventKind{
		cognitionreplay.EventWorkingSetSnapshot,
		cognitionreplay.EventWorkingSetAttached,
		cognitionreplay.EventWorkingSetRetained,
		cognitionreplay.EventWorkingSetTouched,
		cognitionreplay.EventWorkingSetRetained,
		cognitionreplay.EventWorkingSetInvalidated,
		cognitionreplay.EventWorkingSetAttached,
		cognitionreplay.EventWorkingSetScopeClosed,
		cognitionreplay.EventWorkingSetReleased,
		cognitionreplay.EventWorkingSetSnapshot,
	}
	got := make([]cognitionreplay.EventKind, len(state.events))
	for index := range state.events {
		got[index] = state.events[index].Kind
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Working Set command events=%v, want %v", got, want)
	}
	stale := state.entries["working_set\x00working-set-item://lifecycle-first"]
	released := state.entries["working_set\x00working-set-item://lifecycle-second"]
	if stale.Status != cognitionreplay.KnowledgeStale ||
		released.Status != cognitionreplay.KnowledgeReleased {
		t.Fatalf("Working Set final semantic states stale=%+v released=%+v", stale, released)
	}
}

func semanticWorkingSetTestState(started time.Time, finalVersion uint64) *semanticReplayState {
	return newSemanticReplayState(productionTrace{Header: queue.CognitionSealedTracePage{
		EpisodeStartedAt: started, SealedAt: started.Add(time.Second), WorkingSetVersion: finalVersion,
	}}, nil, nil, cognitionpolicy.AttestedBrain{}, cognition.GoalExpression{},
		cognition.CompletionAuthority{}, cognition.ActionCatalog{}, cognition.RuntimeBudget{},
		semanticReplaySupplement{})
}

func semanticAcceptWorkingSnapshot(
	t *testing.T, state *semanticReplayState, ordinal int, at time.Time,
	point string, snapshot workingset.Snapshot,
) {
	t.Helper()
	raw := semanticWorkingSnapshotPayload(t, point, at, snapshot)
	record := semanticWorkingSnapshotRecord(t, point, at, snapshot)
	source := semanticWorkingSource(t, ordinal, record, raw)
	drafts, err := state.mapWorkingSetSnapshot(record, source)
	semanticAppendWorkingDrafts(t, state, source, drafts, err)
}

func semanticAcceptWorkingEvent(
	t *testing.T, state *semanticReplayState, ordinal int, at time.Time, event workingset.Event,
) {
	t.Helper()
	value := queue.CognitionTraceWorkingSetEvent{
		Schema: queue.CognitionTraceWorkingSetEventSchemaV1, CreatedAt: at, Event: event,
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	record := semanticReplayRawRecord("working_set_event", 1, 5, int64(event.Version),
		string(event.SetID)+":event:"+itoa(event.Version), raw)
	source := semanticWorkingSource(t, ordinal, record, raw)
	drafts, mapErr := state.mapWorkingSetEvent(record, source)
	semanticAppendWorkingDrafts(t, state, source, drafts, mapErr)
}

func semanticWorkingSnapshotPayload(
	t *testing.T, point string, at time.Time, snapshot workingset.Snapshot,
) []byte {
	t.Helper()
	raw, err := json.Marshal(queue.CognitionTraceWorkingSetSnapshot{
		Schema: queue.CognitionTraceWorkingSetSnapshotSchemaV1,
		Point:  point, CapturedAt: at, Snapshot: snapshot,
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func semanticWorkingSnapshotRecord(
	t *testing.T, point string, at time.Time, snapshot workingset.Snapshot,
) queue.CognitionSealedTraceRecord {
	t.Helper()
	idPoint := point
	if point == "episode_start" {
		idPoint = "episode-start"
	}
	callOrdinal, phase := int64(0), 1
	if point == "terminal" {
		callOrdinal, phase = 1, 90
	}
	return semanticReplayRawRecord("working_set_snapshot", callOrdinal, phase, int64(snapshot.Version),
		string(snapshot.ID)+":"+idPoint, semanticWorkingSnapshotPayload(t, point, at, snapshot))
}

func semanticWorkingSource(
	t *testing.T, ordinal int, record queue.CognitionSealedTraceRecord, raw []byte,
) cognitionreplay.SourceRecord {
	t.Helper()
	blob, err := cognitionreplay.NewBlob("application/json", raw)
	if err != nil {
		t.Fatal(err)
	}
	return cognitionreplay.SourceRecord{
		Ordinal: uint64(ordinal), CallOrdinal: record.CallOrdinal,
		Phase: record.Phase, Sequence: record.Sequence,
		Kind: record.Kind, ID: record.ID, Payload: blob.Ref(),
	}
}

func semanticEvictingWorkingSet(t *testing.T) (*workingset.Set, workingset.Event, workingset.Event) {
	t.Helper()
	ledgerID, err := taskstate.NewLedgerID(taskstate.LedgerOwner{
		Kind: taskstate.OwnerJob, JobID: 92, RunID: "12345678-9abc-def0-1234-56789abcdef0",
	})
	if err != nil {
		t.Fatal(err)
	}
	set, err := workingset.New(workingset.Owner{LedgerID: ledgerID, JobID: 92, Generation: 1},
		workingset.Budget{MaxItems: 1, MaxBytes: 8, MaxPinnedItems: 0, MaxPinnedBytes: 0})
	if err != nil {
		t.Fatal(err)
	}
	apply := func(name, uri string, expected uint64) workingset.Event {
		id, idErr := workingset.NewCommandID("semantic-eviction", name)
		if idErr != nil {
			t.Fatal(idErr)
		}
		ref := taskstate.Ref{
			URI: uri, Version: "1", Hash: traceTestDigest(name), Relation: taskstate.RefEvidence,
		}
		event, applyErr := set.Apply(workingset.AcquireCommand{
			CommandID: id, ExpectedVersion: expected, Actor: taskstate.AuthorityCode,
			Request: workingset.AcquireRequest{
				ID: workingset.ItemID(name), Ref: ref, Role: workingset.RoleEvidence,
				Retention: workingset.RetentionJob, Scope: set.Scope(), Priority: 10,
				ByteCost: 8, Acquisition: workingset.Acquisition{
					Provider: workingset.ProviderEvidence, OperationID: "operation-" + name,
					Reason: "Required by the semantic replay test.",
				},
			},
		})
		if applyErr != nil {
			t.Fatal(applyErr)
		}
		return event
	}
	return set, apply("first", "evidence://semantic/first", 0),
		apply("second", "evidence://semantic/second", 1)
}

func semanticAppendWorkingDrafts(
	t *testing.T, state *semanticReplayState, source cognitionreplay.SourceRecord,
	drafts []semanticEventDraft, err error,
) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	for _, draft := range drafts {
		if err := state.appendEvent(draft, source); err != nil {
			t.Fatal(err)
		}
	}
}

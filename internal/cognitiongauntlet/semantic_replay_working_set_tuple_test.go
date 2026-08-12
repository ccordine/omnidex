package cognitiongauntlet

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/workingset"
)

func TestSemanticReplayWorkingSetRejectsForgedProductionTuples(t *testing.T) {
	set, event, _ := semanticEvictingWorkingSet(t)
	start, err := workingset.New(set.Owner(), set.Budget())
	if err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, time.August, 12, 5, 0, 0, 0, time.UTC)
	snapshotRaw, err := json.Marshal(semanticRuntimeSnapshot{
		ContextProjection: cognition.ContextProjectionRef{WorkingSetVersion: event.Version},
	})
	if err != nil {
		t.Fatal(err)
	}
	trace := productionTrace{
		Header: queue.CognitionSealedTracePage{
			EpisodeStartedAt: started, SealedAt: started.Add(time.Second),
			WorkingSetVersion: event.Version,
		},
		Records: []queue.CognitionSealedTraceRecord{
			semanticReplayRawRecord("runtime_snapshot", 2, 20, 2, "snapshot", snapshotRaw),
		},
	}
	newState := func() *semanticReplayState {
		state := newSemanticReplayState(trace, nil, nil, cognitionpolicy.AttestedBrain{},
			cognition.GoalExpression{}, cognition.CompletionAuthority{}, cognition.ActionCatalog{},
			cognition.RuntimeBudget{}, semanticReplaySupplement{})
		semanticAcceptWorkingSnapshot(t, state, 1, started, "episode_start", start.Snapshot())
		return state
	}
	eventValue := queue.CognitionTraceWorkingSetEvent{
		Schema:    queue.CognitionTraceWorkingSetEventSchemaV1,
		CreatedAt: started.Add(time.Millisecond), Event: event,
	}
	eventRaw, err := json.Marshal(eventValue)
	if err != nil {
		t.Fatal(err)
	}
	valid := semanticReplayRawRecord("working_set_event", 2, 5, int64(event.Version),
		string(event.SetID)+":event:"+itoa(event.Version), eventRaw)
	if drafts, mapErr := newState().mapWorkingSetEvent(
		valid, semanticWorkingSource(t, 2, valid, eventRaw),
	); mapErr != nil || len(drafts) == 0 {
		t.Fatalf("valid derived Working Set tuple rejected: drafts=%d err=%v", len(drafts), mapErr)
	}
	mutations := map[string]func(*queue.CognitionSealedTraceRecord){
		"call ordinal": func(record *queue.CognitionSealedTraceRecord) { record.CallOrdinal++ },
		"phase":        func(record *queue.CognitionSealedTraceRecord) { record.Phase++ },
		"sequence":     func(record *queue.CognitionSealedTraceRecord) { record.Sequence++ },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			forged := valid
			mutate(&forged)
			if _, mapErr := newState().mapWorkingSetEvent(
				forged, semanticWorkingSource(t, 2, forged, eventRaw),
			); mapErr == nil {
				t.Fatal("forged Working Set event tuple was accepted")
			}
		})
	}
}

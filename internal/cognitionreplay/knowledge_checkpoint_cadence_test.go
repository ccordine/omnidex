package cognitionreplay

import (
	"encoding/json"
	"reflect"
	"strconv"
	"testing"
)

func TestKnowledgeCheckpointCadenceBoundsEveryTerminalPosition(t *testing.T) {
	tests := []struct {
		events int
		want   []uint64
	}{
		{events: 99, want: []uint64{0, 99}},
		{events: 100, want: []uint64{0, 100}},
		{events: 101, want: []uint64{0, 100, 101}},
		{events: 199, want: []uint64{0, 100, 199}},
		{events: 200, want: []uint64{0, 100, 200}},
		{events: 201, want: []uint64{0, 100, 200, 201}},
	}
	for _, test := range tests {
		t.Run(strconv.Itoa(test.events), func(t *testing.T) {
			prepared := prepareCheckpointSeriesForTest(t, test.events, test.want[1:])
			if err := validateKnowledgeCheckpoints(prepared, test.events); err != nil {
				t.Fatal(err)
			}
			if len(prepared) != len(test.want) {
				t.Fatalf("checkpoint count = %d, want %d", len(prepared), len(test.want))
			}
			for index, checkpoint := range prepared {
				if checkpoint.AfterEvent != test.want[index] {
					t.Fatalf("checkpoint %d after event %d, want %d", index+1, checkpoint.AfterEvent, test.want[index])
				}
			}
		})
	}
}

func TestKnowledgeCheckpointCadenceRejectsShiftedBoundaries(t *testing.T) {
	tests := []struct {
		events    int
		endpoints []uint64
	}{
		{events: 101, endpoints: []uint64{1, 101}},
		{events: 199, endpoints: []uint64{99, 199}},
		{events: 201, endpoints: []uint64{1, 101, 201}},
		{events: 201, endpoints: []uint64{99, 199, 201}},
		{events: 201, endpoints: []uint64{100, 101, 201}},
	}
	for _, test := range tests {
		prepared := prepareCheckpointSeriesForTest(t, test.events, test.endpoints)
		if err := validateKnowledgeCheckpoints(prepared, test.events); err == nil {
			t.Fatalf("shifted checkpoint cadence %v was accepted", test.endpoints)
		}
	}
}

func TestKnowledgeCheckpointColdReopenFromEveryIntermediateState(t *testing.T) {
	prepared := prepareCheckpointSeriesForTest(t, 201, []uint64{100, 200, 201})
	if err := validateKnowledgeCheckpoints(prepared, 201); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(prepared)
	if err != nil {
		t.Fatal(err)
	}
	var reopened []KnowledgeCheckpoint
	if err := json.Unmarshal(raw, &reopened); err != nil {
		t.Fatal(err)
	}
	want := reopened[len(reopened)-1].State
	for start := 1; start < len(reopened)-1; start++ {
		state := reopened[start].State
		for index := start + 1; index < len(reopened); index++ {
			state, err = applyKnowledgeDelta(state, *reopened[index].Delta)
			if err != nil {
				t.Fatalf("checkpoint %d cold replay: %v", start+1, err)
			}
		}
		if !reflect.DeepEqual(state, want) {
			t.Fatalf("checkpoint %d cold replay did not reach the terminal state", start+1)
		}
	}
}

func TestKnowledgeCheckpointChainRejectsMissingDuplicateReorderedOrModifiedDelta(t *testing.T) {
	tests := map[string]func([]KnowledgeCheckpoint) []KnowledgeCheckpoint{
		"missing": func(values []KnowledgeCheckpoint) []KnowledgeCheckpoint {
			values = append(values[:1], values[2:]...)
			resequenceCheckpointsForTest(values)
			return values
		},
		"duplicate": func(values []KnowledgeCheckpoint) []KnowledgeCheckpoint {
			duplicate := cloneKnowledgeCheckpoints(values[1:2])
			values = append(values[:2], append(duplicate, values[2:]...)...)
			resequenceCheckpointsForTest(values)
			return values
		},
		"reordered": func(values []KnowledgeCheckpoint) []KnowledgeCheckpoint {
			values[1], values[2] = values[2], values[1]
			resequenceCheckpointsForTest(values)
			return values
		},
		"modified_delta": func(values []KnowledgeCheckpoint) []KnowledgeCheckpoint {
			values[1].Delta.Upserts[0].Status = KnowledgeFailed
			return values
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			values := mutate(checkpointSeriesForTest(t, []uint64{100, 200, 201}))
			prepared, err := prepareKnowledgeCheckpoints(values)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateKnowledgeCheckpoints(prepared, 201); err == nil {
				t.Fatal("mutated checkpoint chain was accepted")
			}
		})
	}
}

func TestKnowledgeCheckpointHashBindsPreviousIntervalAndState(t *testing.T) {
	prepared := prepareCheckpointSeriesForTest(t, 201, []uint64{100, 200, 201})
	mutations := map[string]func([]KnowledgeCheckpoint){
		"previous": func(values []KnowledgeCheckpoint) { values[2].PreviousSHA256 = testDigest("other") },
		"interval": func(values []KnowledgeCheckpoint) { values[2].Delta.ThroughEvent-- },
		"state":    func(values []KnowledgeCheckpoint) { values[2].State.Entries[0].Status = KnowledgeFailed },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			values := cloneKnowledgeCheckpoints(prepared)
			mutate(values)
			if err := validateKnowledgeCheckpoints(values, 201); err == nil {
				t.Fatal("checkpoint hash mutation was accepted")
			}
		})
	}
}

func prepareCheckpointSeriesForTest(t *testing.T, events int, endpoints []uint64) []KnowledgeCheckpoint {
	t.Helper()
	prepared, err := prepareKnowledgeCheckpoints(checkpointSeriesForTest(t, endpoints))
	if err != nil {
		t.Fatal(err)
	}
	if prepared[len(prepared)-1].AfterEvent != uint64(events) {
		t.Fatalf("test checkpoint endpoint %d does not cover %d events", prepared[len(prepared)-1].AfterEvent, events)
	}
	return prepared
}

func checkpointSeriesForTest(t *testing.T, endpoints []uint64) []KnowledgeCheckpoint {
	t.Helper()
	content := testBlob(t, `{"checkpoint":"state"}`).Ref()
	values := []KnowledgeCheckpoint{{
		Sequence: 1, AfterEvent: 0,
		State: KnowledgeState{Schema: KnowledgeStateSchemaV1, Entries: []KnowledgeEntry{}},
	}}
	sourceEvents := make([]uint64, 0, len(endpoints))
	previous := uint64(0)
	for _, endpoint := range endpoints {
		sourceEvents = append(sourceEvents, endpoint)
		entry := KnowledgeEntry{
			Kind: KnowledgeObservation, Ref: "observation://checkpoint-state",
			Status: KnowledgeActive, Authority: AuthorityEnvironment, Content: content,
			SourceEvents: append([]uint64(nil), sourceEvents...),
		}
		values = append(values, KnowledgeCheckpoint{
			Sequence: uint64(len(values) + 1), AfterEvent: endpoint,
			State: KnowledgeState{Schema: KnowledgeStateSchemaV1, Entries: []KnowledgeEntry{entry}},
			Delta: &KnowledgeDelta{
				Schema: KnowledgeDeltaSchemaV1, FromEvent: previous + 1, ThroughEvent: endpoint,
				Upserts: []KnowledgeEntry{entry}, Releases: []KnowledgeRelease{},
			},
		})
		previous = endpoint
	}
	return values
}

func resequenceCheckpointsForTest(values []KnowledgeCheckpoint) {
	for index := range values {
		values[index].Sequence = uint64(index + 1)
	}
}

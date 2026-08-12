package queue

import "testing"

func TestVerifyCognitionWorkingSetTraceTupleUsesProductionProjectionChronology(t *testing.T) {
	points := []CognitionProjectionWorkingVersion{
		{CallOrdinal: 1, WorkingVersion: 2},
		{CallOrdinal: 3, WorkingVersion: 5},
	}
	tests := []struct {
		name    string
		record  CognitionSealedTraceRecord
		point   CognitionWorkingSetTracePoint
		version uint64
	}{
		{"start", CognitionSealedTraceRecord{Kind: "working_set_snapshot", Phase: 1}, CognitionWorkingSetTraceEpisodeStart, 0},
		{"event at first projection", CognitionSealedTraceRecord{Kind: "working_set_event", CallOrdinal: 1, Phase: 5, Sequence: 2}, CognitionWorkingSetTraceEvent, 2},
		{"event at later projection", CognitionSealedTraceRecord{Kind: "working_set_event", CallOrdinal: 3, Phase: 5, Sequence: 3}, CognitionWorkingSetTraceEvent, 3},
		{"event after projections", CognitionSealedTraceRecord{Kind: "working_set_event", CallOrdinal: 4, Phase: 5, Sequence: 6}, CognitionWorkingSetTraceEvent, 6},
		{"terminal", CognitionSealedTraceRecord{Kind: "working_set_snapshot", CallOrdinal: 4, Phase: 90, Sequence: 6}, CognitionWorkingSetTraceTerminal, 6},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := VerifyCognitionWorkingSetTraceTuple(test.record, test.point, test.version, points); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestVerifyCognitionWorkingSetTraceTupleRejectsTupleAndChronologyChanges(t *testing.T) {
	valid := CognitionSealedTraceRecord{
		Kind: "working_set_event", CallOrdinal: 2, Phase: 5, Sequence: 3,
	}
	points := []CognitionProjectionWorkingVersion{{CallOrdinal: 2, WorkingVersion: 3}}
	mutations := map[string]func(*CognitionSealedTraceRecord){
		"kind":     func(record *CognitionSealedTraceRecord) { record.Kind = "working_set_snapshot" },
		"call":     func(record *CognitionSealedTraceRecord) { record.CallOrdinal++ },
		"phase":    func(record *CognitionSealedTraceRecord) { record.Phase++ },
		"sequence": func(record *CognitionSealedTraceRecord) { record.Sequence++ },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			forged := valid
			mutate(&forged)
			if VerifyCognitionWorkingSetTraceTuple(forged, CognitionWorkingSetTraceEvent, 3, points) == nil {
				t.Fatal("changed Working Set trace tuple was accepted")
			}
		})
	}
	if VerifyCognitionWorkingSetTraceTuple(valid, CognitionWorkingSetTraceEvent, 3,
		[]CognitionProjectionWorkingVersion{{CallOrdinal: 2, WorkingVersion: 3}, {CallOrdinal: 1, WorkingVersion: 4}},
	) == nil {
		t.Fatal("noncanonical projection chronology was accepted")
	}
}

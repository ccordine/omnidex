package queue

import "testing"

func TestVerifyCognitionSealedTraceRecordOrderUsesProductionAuthority(t *testing.T) {
	first := CognitionSealedTraceRecord{
		Kind: "transition", CallOrdinal: 0, Phase: 10, Sequence: 1, ID: "transition-a",
	}
	second := CognitionSealedTraceRecord{
		Kind: "obligation_graph", CallOrdinal: 0, Phase: 20, Sequence: 1, ID: "graph-a",
	}
	if err := VerifyCognitionSealedTraceRecordOrder([]CognitionSealedTraceRecord{first, second}); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCognitionSealedTraceRecordOrder([]CognitionSealedTraceRecord{second, first}); err == nil {
		t.Fatal("production trace order verifier accepted a reversed record pair")
	}
	if err := VerifyCognitionSealedTraceRecordOrder([]CognitionSealedTraceRecord{
		first,
		{Kind: "obligation_graph", CallOrdinal: 0, Phase: 20, Sequence: 1, ID: first.ID},
		first,
	}); err == nil {
		t.Fatal("production trace order verifier accepted a duplicate kind/ID authority")
	}
}

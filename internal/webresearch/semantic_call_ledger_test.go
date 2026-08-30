package webresearch

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/websearch"
)

func TestSemanticCallLedgerPreservesFullyRestoredAndMixedProvenance(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		name     string
		receipts []SemanticCallReceipt
		want     SemanticCallReceipt
	}{
		{
			name: "fully restored",
			receipts: []SemanticCallReceipt{
				{Reused: true}, {Reused: true},
			},
			want: SemanticCallReceipt{Reused: true},
		},
		{
			name: "restored and fresh",
			receipts: []SemanticCallReceipt{
				{Reused: true}, {Calls: 1},
			},
			want: SemanticCallReceipt{Calls: 1},
		},
	} {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			var ledger SemanticCallLedger
			for index, receipt := range fixture.receipts {
				if err := ledger.Record(
					"semantic leaf "+string(rune('a'+index)), receipt, 1,
				); err != nil {
					t.Fatal(err)
				}
			}
			got, err := ledger.ValidateForMaximum("test semantic work", len(fixture.receipts))
			if err != nil {
				t.Fatal(err)
			}
			if got != fixture.want {
				t.Fatalf("receipt=%+v want %+v", got, fixture.want)
			}
		})
	}
}

func TestSemanticCallLedgerRejectsAbsentAndForgedProvenance(t *testing.T) {
	t.Parallel()
	if _, err := (SemanticCallLedger{}).TotalForSuccess(); err == nil {
		t.Fatal("empty web semantic ledger was accepted")
	}
	for _, receipt := range []SemanticCallReceipt{
		{},
		{Calls: 1, Reused: true},
		{Calls: 2},
	} {
		var ledger SemanticCallLedger
		if err := ledger.Record("invalid semantic leaf", receipt, 1); err == nil {
			t.Fatalf("invalid receipt %+v was accepted", receipt)
		}
	}
	corrupt := SemanticCallLedger{entries: []semanticCallLedgerEntry{{
		label: "corrupt semantic leaf", receipt: SemanticCallReceipt{}, maximumCalls: 1,
	}}}
	if _, err := corrupt.TotalForSuccess(); err == nil {
		t.Fatal("corrupt stored web semantic receipt was accepted")
	}
	var oversized SemanticCallLedger
	if err := oversized.Record(
		"oversized aggregate", SemanticCallReceipt{Calls: 1}, 2,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := oversized.ValidateForMaximum("bounded aggregate", 1); err == nil {
		t.Fatal("aggregate receipt provenance exceeded its exact upper bound")
	}
}

func TestWebMachineAcceptsFullyRestoredAndMixedSemanticLedgers(t *testing.T) {
	for _, fixture := range []struct {
		name      string
		relevance SemanticCallReceipt
		synthesis SemanticCallReceipt
		want      SemanticCallReceipt
	}{
		{
			name:      "fully restored",
			relevance: SemanticCallReceipt{Reused: true},
			synthesis: SemanticCallReceipt{Reused: true},
			want:      SemanticCallReceipt{Reused: true},
		},
		{
			name:      "restored relevance and fresh synthesis",
			relevance: SemanticCallReceipt{Reused: true},
			synthesis: SemanticCallReceipt{Calls: 1},
			want:      SemanticCallReceipt{Calls: 1},
		},
	} {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			candidate := candidateFixture("https://evidence.example/reuse", "Reusable evidence")
			document := documentFixture(
				candidate.URL, candidate.Title, "The bounded evidence supports the answer.",
			)
			acquisition := &scriptedAcquisition{
				discoveries: map[string]discoverOutcome{
					"reusable evidence": {report: candidateReport("reusable evidence", candidate)},
				},
				documents: map[websearch.CandidateID]websearch.Document{candidate.ID: document},
			}
			var relevanceLedger SemanticCallLedger
			if err := relevanceLedger.Record(
				"relevance leaf", fixture.relevance, 1,
			); err != nil {
				t.Fatal(err)
			}
			var synthesisLedger SemanticCallLedger
			if err := synthesisLedger.Record(
				"synthesis leaf", fixture.synthesis, 1,
			); err != nil {
				t.Fatal(err)
			}
			machine := newFixtureMachine(
				t,
				Objective{
					ID: "objective_reuse", Question: "What does the evidence support?",
					InitialQuery: "reusable evidence", Status: ObjectivePending,
				},
				acquisition,
				&recordingRelevanceStation{decision: RelevanceDecision{
					Outcome: RelevanceSelected, CandidateIDs: []websearch.CandidateID{candidate.ID},
					SemanticCalls: fixture.relevance.Calls, CallLedger: relevanceLedger,
				}},
				&recordingSynthesisStation{decision: GroundedSynthesisDecision{
					Paragraphs: []GroundedParagraph{{
						Text:        "The bounded evidence supports the answer.",
						EvidenceIDs: []EvidenceID{evidenceID(document.ID)},
					}},
					SemanticCalls: fixture.synthesis.Calls, CallLedger: synthesisLedger,
				}},
				2_000,
			)
			result, err := machine.Run(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := result.CallLedger.ValidateForMaximum(
				"completed web result", 2,
			)
			if err != nil {
				t.Fatal(err)
			}
			if receipt != fixture.want || result.SemanticCalls != fixture.want.Calls || !result.Complete {
				t.Fatalf("receipt=%+v result=%+v", receipt, result)
			}
		})
	}
}

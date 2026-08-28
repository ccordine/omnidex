package webresearch

import (
	"context"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/websearch"
)

func TestGatherRelevantEvidenceUsesTermsCodeOwnedAcquisitionAndIDOnlyRelevance(t *testing.T) {
	t.Parallel()
	irrelevant := candidateFixture("https://example.test/bread", "Bread notes")
	relevant := candidateFixture("https://example.test/mars", "Mars orbit")
	irrelevantDocument := documentFixture(irrelevant.URL, irrelevant.Title, "Steam delays crust setting.")
	relevantDocument := documentFixture(relevant.URL, relevant.Title, "Mars circles the Sun in about 687 Earth days.")
	acquisition := &scriptedAcquisition{
		discoveries: map[string]discoverOutcome{
			"Mars orbital period": {report: candidateReport("Mars orbital period", irrelevant, relevant)},
		},
		documents: map[websearch.CandidateID]websearch.Document{
			irrelevant.ID: irrelevantDocument,
			relevant.ID:   relevantDocument,
		},
	}
	terms := &recordingTermsStation{decision: SearchTermsDecision{
		Terms: []string{"Mars orbital period"}, SemanticCalls: 3,
	}}
	relevance := &recordingRelevanceStation{decision: RelevanceDecision{
		Outcome: RelevanceSelected, CandidateIDs: []websearch.CandidateID{relevant.ID},
		SemanticCalls: 2,
	}}

	result, err := GatherRelevantEvidence(
		context.Background(),
		EvidenceRequest{ID: "roleplay_research", Question: "How long is a Martian year?"},
		EvidenceConfig{
			MaxSearchTerms: 3, MaxSearchTermBytes: 256, MaxFetchCandidates: 2,
			MaxProjectionBytes: 8 * 1024, MaxRelevantCandidates: 2,
			CandidateSummaryBytes: 512,
		},
		acquisition, terms, relevance,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.SearchTermsCalls != 1 || result.RelevanceCalls != 1 || result.SemanticCalls != 5 ||
		terms.calls != 1 || relevance.calls != 1 {
		t.Fatalf("semantic calls terms=%d/%d relevance=%d/%d raw=%d",
			result.SearchTermsCalls, terms.calls, result.RelevanceCalls, relevance.calls,
			result.SemanticCalls)
	}
	if !reflect.DeepEqual(acquisition.events, []string{"discover:Mars orbital period", "fetch:2"}) {
		t.Fatalf("code-owned acquisition events=%v", acquisition.events)
	}
	if len(result.Evidence) != 1 || result.Evidence[0].CandidateID != relevant.ID ||
		len(result.Projected) != 1 || result.Projected[0].CandidateID != relevant.ID {
		t.Fatalf("selected bounded evidence=%#v projected=%#v", result.Evidence, result.Projected)
	}
	if len(relevance.last.Candidates) != 2 || relevance.last.Candidates[0].CandidateID != irrelevant.ID ||
		relevance.last.Candidates[1].CandidateID != relevant.ID {
		t.Fatalf("relevance did not receive the bounded code-acquired set: %#v", relevance.last.Candidates)
	}
}

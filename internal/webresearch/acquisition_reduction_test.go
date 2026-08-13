package webresearch

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/websearch"
)

func TestInitialDiscoveryDeterministicallyReducesCandidatesBeforeFetch(t *testing.T) {
	candidates, documents := candidateSetFixture(16, "initial")
	acquisition := &scriptedAcquisition{
		discoveries: map[string]discoverOutcome{
			"bounded initial": {report: candidateReport("bounded initial", candidates...)},
		},
		documents: documents,
	}
	terms := &recordingTermsStation{decision: SearchTermsDecision{Terms: []string{"must not run"}}}
	relevance := &recordingRelevanceStation{decision: RelevanceDecision{
		Outcome: RelevanceSelected, CandidateIDs: []websearch.CandidateID{candidates[0].ID},
	}}
	synthesis := &recordingSynthesisStation{decision: GroundedSynthesisDecision{Paragraphs: []GroundedParagraph{{
		Text:        "The first bounded source answers the question.",
		EvidenceIDs: []EvidenceID{evidenceID(documents[candidates[0].ID].ID)},
	}}}}
	machine := boundedCandidateMachine(t, "bounded initial", acquisition, terms, relevance, synthesis)

	result, err := machine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || terms.calls != 0 || relevance.calls != 1 || synthesis.calls != 1 {
		t.Fatalf("result=%#v terms=%d relevance=%d synthesis=%d", result, terms.calls, relevance.calls, synthesis.calls)
	}
	if got := strings.Join(acquisition.events, ","); got != "discover:bounded initial,fetch:8" {
		t.Fatalf("acquisition order=%q", got)
	}
}

func TestExpandedDiscoveryDeterministicallyReducesCandidatesBeforeFetch(t *testing.T) {
	first, firstDocuments := candidateSetFixture(6, "first")
	second, secondDocuments := candidateSetFixture(6, "second")
	documents := firstDocuments
	for id, document := range secondDocuments {
		documents[id] = document
	}
	acquisition := &scriptedAcquisition{
		discoveries: map[string]discoverOutcome{
			"unresolved":  {report: emptyCandidateReport("unresolved"), err: websearch.ErrNoCandidates},
			"first term":  {report: candidateReport("first term", first...)},
			"second term": {report: candidateReport("second term", second...)},
		},
		documents: documents,
	}
	terms := &recordingTermsStation{decision: SearchTermsDecision{Terms: []string{"first term", "second term"}}}
	relevance := &recordingRelevanceStation{decision: RelevanceDecision{
		Outcome: RelevanceSelected, CandidateIDs: []websearch.CandidateID{first[0].ID},
	}}
	synthesis := &recordingSynthesisStation{decision: GroundedSynthesisDecision{Paragraphs: []GroundedParagraph{{
		Text:        "The first expanded source answers the question.",
		EvidenceIDs: []EvidenceID{evidenceID(documents[first[0].ID].ID)},
	}}}}
	machine := boundedCandidateMachine(t, "unresolved", acquisition, terms, relevance, synthesis)

	result, err := machine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || terms.calls != 1 || relevance.calls != 1 || synthesis.calls != 1 {
		t.Fatalf("result=%#v terms=%d relevance=%d synthesis=%d", result, terms.calls, relevance.calls, synthesis.calls)
	}
	want := "discover:unresolved,discover:first term,discover:second term,fetch:8"
	if got := strings.Join(acquisition.events, ","); got != want {
		t.Fatalf("acquisition order=%q want %q", got, want)
	}
}

func boundedCandidateMachine(
	t *testing.T,
	query string,
	acquisition Acquisition,
	terms SearchTermsStation,
	relevance RelevanceStation,
	synthesis GroundedSynthesisStation,
) *Machine {
	t.Helper()
	machine, err := New(Objective{
		ID: "objective_candidate_reduction", Question: "Which bounded source answers the question?", InitialQuery: query,
		Acceptance: exactAcceptance(), Status: ObjectivePending,
	}, Config{
		MaxSearchTerms: 3, MaxSearchTermBytes: 120, MaxFetchCandidates: 8,
		MaxProjectionBytes: 8_000, MaxRelevantCandidates: 3, CandidateSummaryBytes: 240,
		MaxSynthesisParagraphs: 4, MaxSynthesisParagraphBytes: 1_000,
	}, acquisition, terms, relevance, synthesis, &recordingSynthesisCorrectionStation{}, &recordingClaimEvidenceReviewStation{})
	if err != nil {
		t.Fatal(err)
	}
	return machine
}

func candidateSetFixture(count int, prefix string) ([]websearch.Candidate, map[websearch.CandidateID]websearch.Document) {
	candidates := make([]websearch.Candidate, 0, count)
	documents := make(map[websearch.CandidateID]websearch.Document, count)
	for index := 0; index < count; index++ {
		title := fmt.Sprintf("%s source %02d", prefix, index)
		candidate := candidateFixture(fmt.Sprintf("https://%s-%02d.example/evidence", prefix, index), title)
		candidates = append(candidates, candidate)
		documents[candidate.ID] = documentFixture(candidate.URL, title, title+" evidence.")
	}
	return candidates, documents
}

package webresearch

import (
	"context"
	"errors"
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
	relevance := &recordingRelevanceStation{decision: RelevanceDecision{
		Outcome: RelevanceSelected, CandidateIDs: []websearch.CandidateID{candidates[0].ID},
	}}
	synthesis := &recordingSynthesisStation{decision: GroundedSynthesisDecision{Paragraphs: []GroundedParagraph{{
		Text:        "The first bounded source answers the question.",
		EvidenceIDs: []EvidenceID{evidenceID(documents[candidates[0].ID].ID)},
	}}}}
	machine := boundedCandidateMachine(t, "bounded initial", acquisition, relevance, synthesis)

	result, err := machine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || relevance.calls != 1 || synthesis.calls != 1 {
		t.Fatalf("result=%#v relevance=%d synthesis=%d", result, relevance.calls, synthesis.calls)
	}
	if got := strings.Join(acquisition.events, ","); got != "discover:bounded initial,fetch:8" {
		t.Fatalf("acquisition order=%q", got)
	}
}

func TestEmptyExactDiscoveryNeverExpandsToModelAuthoredQueries(t *testing.T) {
	acquisition := &scriptedAcquisition{
		discoveries: map[string]discoverOutcome{
			"unresolved": {report: emptyCandidateReport("unresolved"), err: websearch.ErrNoCandidates},
		},
		documents: map[websearch.CandidateID]websearch.Document{},
	}
	relevance := &recordingRelevanceStation{}
	synthesis := &recordingSynthesisStation{}
	machine := boundedCandidateMachine(t, "unresolved", acquisition, relevance, synthesis)

	result, err := machine.Run(context.Background())
	if !errors.Is(err, ErrEvidenceUnavailable) {
		t.Fatalf("Run error=%v want ErrEvidenceUnavailable", err)
	}
	if result.Complete || relevance.calls != 0 || synthesis.calls != 0 {
		t.Fatalf("result=%#v relevance=%d synthesis=%d", result, relevance.calls, synthesis.calls)
	}
	if got := strings.Join(acquisition.events, ","); got != "discover:unresolved" {
		t.Fatalf("acquisition used anything beyond exact query: %q", got)
	}
}

func boundedCandidateMachine(
	t *testing.T,
	query string,
	acquisition Acquisition,
	relevance RelevanceStation,
	synthesis GroundedSynthesisStation,
) *Machine {
	t.Helper()
	machine, err := New(Objective{
		ID: "objective_candidate_reduction", Question: "Which bounded source answers the question?", InitialQuery: query, Status: ObjectivePending,
	}, Config{
		MaxFetchCandidates: 8,
		MaxProjectionBytes: 8_000, MaxRelevantCandidates: 3, CandidateSummaryBytes: 240,
		MaxSynthesisParagraphs: 4, MaxSynthesisParagraphBytes: 1_000,
	}, acquisition, relevance, synthesis)
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

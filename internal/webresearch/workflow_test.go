package webresearch

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/websearch"
)

func TestDeterministicInitialAcquisitionStillRequiresBoundedRelevance(t *testing.T) {
	oceanCandidate := candidateFixture("https://science.example/ocean-current", "Ocean circulation")
	oceanDocument := documentFixture(
		"https://science.example/ocean-current", "Ocean circulation", "Observed circulation is driven by wind and density differences.",
	)
	acquisition := &scriptedAcquisition{
		discoveries: map[string]discoverOutcome{
			"ocean current circulation": {report: candidateReport("ocean current circulation", oceanCandidate)},
		},
		documents: map[websearch.CandidateID]websearch.Document{
			oceanCandidate.ID: oceanDocument,
		},
	}
	terms := &recordingTermsStation{decision: SearchTermsDecision{Terms: []string{"must not run"}}}
	relevance := &recordingRelevanceStation{decision: RelevanceDecision{
		Outcome: RelevanceSelected, CandidateIDs: []websearch.CandidateID{oceanCandidate.ID},
	}}
	synthesis := &recordingSynthesisStation{decision: GroundedSynthesisDecision{Paragraphs: []GroundedParagraph{
		{Text: "Ocean circulation is driven by wind and water-density differences.", EvidenceIDs: []EvidenceID{evidenceID(oceanDocument.ID)}},
	}}}
	machine := newFixtureMachine(t, Objective{
		ID: "objective_ocean", Question: "What drives ocean circulation?", InitialQuery: "ocean current circulation",
		Acceptance: exactAcceptance(), Status: ObjectivePending,
	}, acquisition, terms, relevance, synthesis, 4_000)

	result, err := machine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || result.Objective.Status != ObjectiveComplete {
		t.Fatalf("result did not complete: %#v", result)
	}
	if result.SearchTermsCalls != 0 || result.RelevanceCalls != 1 || result.SynthesisCalls != 1 || result.ClaimEvidenceReviewCalls != 1 {
		t.Fatalf("station counters terms=%d relevance=%d synthesis=%d", result.SearchTermsCalls, result.RelevanceCalls, result.SynthesisCalls)
	}
	if result.AcquisitionAttempts != 2 || result.DiscoveryAttempts != 1 || result.FetchAttempts != 1 || result.AcquisitionAttemptLimit != 6 {
		t.Fatalf("acquisition attempts=%d discovery=%d fetch=%d limit=%d", result.AcquisitionAttempts, result.DiscoveryAttempts, result.FetchAttempts, result.AcquisitionAttemptLimit)
	}
	if terms.calls != 0 || relevance.calls != 1 || synthesis.calls != 1 {
		t.Fatalf("fake calls terms=%d relevance=%d synthesis=%d", terms.calls, relevance.calls, synthesis.calls)
	}
	if got := strings.Join(acquisition.events, ","); got != "discover:ocean current circulation,fetch:1" {
		t.Fatalf("acquisition order=%s", got)
	}
	if !strings.Contains(result.Artifact.Rendered, "[1]") || !strings.Contains(result.Artifact.Rendered, "https://science.example/ocean-current") {
		t.Fatalf("code did not render exact citation source: %s", result.Artifact.Rendered)
	}
}

func TestTermAndRelevanceStationsCrossOnlyTheirNamedBounds(t *testing.T) {
	firstCandidate := candidateFixture("https://city.example/library/schedule", "Library schedule")
	secondCandidate := candidateFixture("https://community.example/library/notice", "Community notice")
	firstDocument := documentFixture(firstCandidate.URL, firstCandidate.Title, strings.Repeat("Official schedule evidence. ", 45))
	secondDocument := documentFixture(secondCandidate.URL, secondCandidate.Title, strings.Repeat("Community announcement. ", 45))
	acquisition := &scriptedAcquisition{
		discoveries: map[string]discoverOutcome{
			"central library weekend availability": {report: emptyCandidateReport("central library weekend availability"), err: websearch.ErrNoCandidates},
			"library Sunday hours":                 {report: candidateReport("library Sunday hours", firstCandidate)},
			"municipal weekend opening":            {report: candidateReport("municipal weekend opening", secondCandidate)},
		},
		documents: map[websearch.CandidateID]websearch.Document{
			firstCandidate.ID:  firstDocument,
			secondCandidate.ID: secondDocument,
		},
	}
	terms := &recordingTermsStation{decision: SearchTermsDecision{Terms: []string{"library Sunday hours", "municipal weekend opening"}}}
	relevance := &recordingRelevanceStation{decision: RelevanceDecision{
		Outcome: RelevanceSelected, CandidateIDs: []websearch.CandidateID{firstCandidate.ID},
	}}
	synthesis := &recordingSynthesisStation{decision: GroundedSynthesisDecision{Paragraphs: []GroundedParagraph{
		{Text: "The central library is open on Sunday.", EvidenceIDs: []EvidenceID{evidenceID(firstDocument.ID)}},
	}}}
	machine := newFixtureMachine(t, Objective{
		ID: "objective_library", Question: "Is the central library open on Sunday?", InitialQuery: "central library weekend availability",
		Acceptance: exactAcceptance(), Status: ObjectivePending,
	}, acquisition, terms, relevance, synthesis, 600)

	result, err := machine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || result.SearchTermsCalls != 1 || result.RelevanceCalls != 1 || result.SynthesisCalls != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.AcquisitionAttempts != 4 || result.DiscoveryAttempts != 3 || result.FetchAttempts != 1 || result.AcquisitionAttemptLimit != 6 {
		t.Fatalf("acquisition attempts=%d discovery=%d fetch=%d limit=%d", result.AcquisitionAttempts, result.DiscoveryAttempts, result.FetchAttempts, result.AcquisitionAttemptLimit)
	}
	wantSteps := []Step{StepInitialDiscovery, StepSearchTermsResolved, StepExpandedDiscovery, StepDocumentsFetched, StepRelevanceResolved, StepEvidenceProjected, StepSynthesisResolved, StepClaimEvidenceReviewed, StepObjectiveCompleted}
	if fmt.Sprint(result.Steps) != fmt.Sprint(wantSteps) {
		t.Fatalf("steps=%v want %v", result.Steps, wantSteps)
	}
	if len(synthesis.last.Evidence) != 1 || synthesis.last.Evidence[0].CandidateID != firstCandidate.ID {
		t.Fatalf("synthesis projection=%#v", synthesis.last.Evidence)
	}
	if len(relevance.last.Candidates) != 2 {
		t.Fatalf("relevance candidates=%d want 2", len(relevance.last.Candidates))
	}
	if terms.last.Question != "Is the central library open on Sunday?" || len(terms.last.AttemptedQueries) != 1 {
		t.Fatalf("term call=%#v", terms.last)
	}
	if terms.last.MaxTerms != 3 || terms.last.MaxTermBytes != 120 {
		t.Fatalf("term bounds=%#v", terms.last)
	}
	if relevance.last.MaxSelections != 2 {
		t.Fatalf("relevance bound=%d", relevance.last.MaxSelections)
	}
	if synthesis.last.MaxParagraphs != 4 || synthesis.last.MaxParagraphBytes != 1_000 {
		t.Fatalf("synthesis bounds=%#v", synthesis.last)
	}
}

func TestInitialUsableEvidenceNeverCallsSearchTermsEvenWhenProjectionOverflows(t *testing.T) {
	first := candidateFixture("https://alpha.example/doc", "Alpha")
	second := candidateFixture("https://beta.example/doc", "Beta")
	firstDocument := documentFixture(first.URL, first.Title, strings.Repeat("alpha ", 200))
	secondDocument := documentFixture(second.URL, second.Title, strings.Repeat("beta ", 200))
	acquisition := &scriptedAcquisition{
		discoveries: map[string]discoverOutcome{"bounded evidence": {report: candidateReport("bounded evidence", first, second)}},
		documents: map[websearch.CandidateID]websearch.Document{
			first.ID:  firstDocument,
			second.ID: secondDocument,
		},
	}
	terms := &recordingTermsStation{decision: SearchTermsDecision{Terms: []string{"illegal"}}}
	relevance := &recordingRelevanceStation{decision: RelevanceDecision{
		Outcome: RelevanceSelected, CandidateIDs: []websearch.CandidateID{second.ID},
	}}
	synthesis := &recordingSynthesisStation{decision: GroundedSynthesisDecision{Paragraphs: []GroundedParagraph{
		{Text: "Beta is relevant.", EvidenceIDs: []EvidenceID{evidenceID(secondDocument.ID)}},
	}}}
	machine := newFixtureMachine(t, Objective{
		ID: "objective_bounds", Question: "Which evidence is relevant?", InitialQuery: "bounded evidence",
		Acceptance: exactAcceptance(), Status: ObjectivePending,
	}, acquisition, terms, relevance, synthesis, 500)
	result, err := machine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.SearchTermsCalls != 0 || terms.calls != 0 || result.RelevanceCalls != 1 {
		t.Fatalf("calls=%#v terms=%d", result, terms.calls)
	}
}

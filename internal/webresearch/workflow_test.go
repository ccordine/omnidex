package webresearch

import (
	"context"
	"errors"
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
	relevance := &recordingRelevanceStation{decision: RelevanceDecision{
		Outcome: RelevanceSelected, CandidateIDs: []websearch.CandidateID{oceanCandidate.ID},
	}}
	synthesis := &recordingSynthesisStation{decision: GroundedSynthesisDecision{Paragraphs: []GroundedParagraph{
		{Text: "Ocean circulation is driven by wind and water-density differences.", EvidenceIDs: []EvidenceID{evidenceID(oceanDocument.ID)}},
	}}}
	machine := newFixtureMachine(t, Objective{
		ID: "objective_ocean", Question: "What drives ocean circulation?", InitialQuery: "ocean current circulation", Status: ObjectivePending,
	}, acquisition, relevance, synthesis, 4_000)

	result, err := machine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || result.Objective.Status != ObjectiveComplete {
		t.Fatalf("result did not complete: %#v", result)
	}
	if result.RelevanceCalls != 1 || result.SynthesisCalls != 1 {
		t.Fatalf("station counters relevance=%d synthesis=%d", result.RelevanceCalls, result.SynthesisCalls)
	}
	if result.AcquisitionAttempts != 2 || result.DiscoveryAttempts != 1 || result.FetchAttempts != 1 || result.AcquisitionAttemptLimit != 2 {
		t.Fatalf("acquisition attempts=%d discovery=%d fetch=%d limit=%d", result.AcquisitionAttempts, result.DiscoveryAttempts, result.FetchAttempts, result.AcquisitionAttemptLimit)
	}
	if relevance.calls != 1 || synthesis.calls != 1 {
		t.Fatalf("fake calls relevance=%d synthesis=%d", relevance.calls, synthesis.calls)
	}
	if got := strings.Join(acquisition.events, ","); got != "discover:ocean current circulation,fetch:1" {
		t.Fatalf("acquisition order=%s", got)
	}
	if !strings.Contains(result.Artifact.Rendered, "[1]") || !strings.Contains(result.Artifact.Rendered, "https://science.example/ocean-current") {
		t.Fatalf("code did not render exact citation source: %s", result.Artifact.Rendered)
	}
}

func TestExactInitialQueryExhaustionFailsWithoutSemanticQueryExpansion(t *testing.T) {
	acquisition := &scriptedAcquisition{
		discoveries: map[string]discoverOutcome{
			"central library weekend availability": {report: emptyCandidateReport("central library weekend availability"), err: websearch.ErrNoCandidates},
		},
		documents: map[websearch.CandidateID]websearch.Document{},
	}
	relevance := &recordingRelevanceStation{}
	synthesis := &recordingSynthesisStation{}
	machine := newFixtureMachine(t, Objective{
		ID: "objective_library", Question: "Is the central library open on Sunday?", InitialQuery: "central library weekend availability", Status: ObjectivePending,
	}, acquisition, relevance, synthesis, 600)

	result, err := machine.Run(context.Background())
	if !errors.Is(err, ErrEvidenceUnavailable) {
		t.Fatalf("Run error=%v want ErrEvidenceUnavailable", err)
	}
	if result.Complete || result.RelevanceCalls != 0 || result.SynthesisCalls != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.AcquisitionAttempts != 1 || result.DiscoveryAttempts != 1 || result.FetchAttempts != 0 || result.AcquisitionAttemptLimit != 2 {
		t.Fatalf("acquisition attempts=%d discovery=%d fetch=%d limit=%d", result.AcquisitionAttempts, result.DiscoveryAttempts, result.FetchAttempts, result.AcquisitionAttemptLimit)
	}
	if got := strings.Join(acquisition.events, ","); got != "discover:central library weekend availability" {
		t.Fatalf("query authority changed: %q", got)
	}
}

func TestInitialUsableEvidenceUsesRelevanceWhenProjectionOverflows(t *testing.T) {
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
	relevance := &recordingRelevanceStation{decision: RelevanceDecision{
		Outcome: RelevanceSelected, CandidateIDs: []websearch.CandidateID{second.ID},
	}}
	synthesis := &recordingSynthesisStation{decision: GroundedSynthesisDecision{Paragraphs: []GroundedParagraph{
		{Text: "Beta is relevant.", EvidenceIDs: []EvidenceID{evidenceID(secondDocument.ID)}},
	}}}
	machine := newFixtureMachine(t, Objective{
		ID: "objective_bounds", Question: "Which evidence is relevant?", InitialQuery: "bounded evidence", Status: ObjectivePending,
	}, acquisition, relevance, synthesis, 500)
	result, err := machine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.RelevanceCalls != 1 {
		t.Fatalf("calls=%#v", result)
	}
}

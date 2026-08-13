package webresearch

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/websearch"
)

func TestInfrastructureFailureDoesNotBecomeSearchTermInference(t *testing.T) {
	acquisition := &scriptedAcquisition{discoveries: map[string]discoverOutcome{
		"service status": {
			report: websearch.CandidateReport{Query: "service status", Diagnostics: []websearch.ProviderDiagnostic{{Provider: websearch.ProviderGoogle, Outcome: websearch.DiscoveryFailed, Failure: "network down"}}},
			err:    websearch.ErrNoCandidates,
		},
	}}
	terms := &recordingTermsStation{decision: SearchTermsDecision{Terms: []string{"different words"}}}
	machine := newFixtureMachine(t, Objective{
		ID: "objective_failure", Question: "What is the service status?", InitialQuery: "service status",
		Acceptance: exactAcceptance(), Status: ObjectivePending,
	}, acquisition, terms, &recordingRelevanceStation{}, &recordingSynthesisStation{}, 2_000)
	result, err := machine.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "every provider failed") {
		t.Fatalf("Run error=%v", err)
	}
	if result.SearchTermsCalls != 0 || terms.calls != 0 {
		t.Fatalf("infrastructure failure invoked semantic station: result=%d fake=%d", result.SearchTermsCalls, terms.calls)
	}
}

func TestStationOutputsFailLoudly(t *testing.T) {
	candidate := candidateFixture("https://evidence.example/doc", "Evidence")
	tests := []struct {
		name      string
		build     func(*scriptedAcquisition) (SearchTermsStation, RelevanceStation, GroundedSynthesisStation)
		query     string
		budget    int
		wantError string
	}{
		{
			name: "duplicate search terms",
			build: func(acquisition *scriptedAcquisition) (SearchTermsStation, RelevanceStation, GroundedSynthesisStation) {
				acquisition.discoveries["initial empty"] = discoverOutcome{report: emptyCandidateReport("initial empty"), err: websearch.ErrNoCandidates}
				return &recordingTermsStation{decision: SearchTermsDecision{Terms: []string{"same", "same"}}}, &recordingRelevanceStation{}, &recordingSynthesisStation{}
			},
			query: "initial empty", budget: 2_000, wantError: "duplicate search term",
		},
		{
			name: "unknown relevance candidate",
			build: func(acquisition *scriptedAcquisition) (SearchTermsStation, RelevanceStation, GroundedSynthesisStation) {
				acquisition.discoveries["overflow"] = discoverOutcome{report: candidateReport("overflow", candidate)}
				acquisition.documents[candidate.ID] = documentFixture(candidate.URL, candidate.Title, strings.Repeat("overflow ", 300))
				return &recordingTermsStation{}, &recordingRelevanceStation{decision: RelevanceDecision{
					Outcome: RelevanceSelected, CandidateIDs: []websearch.CandidateID{"candidate_unknown"},
				}}, &recordingSynthesisStation{}
			},
			query: "overflow", budget: 500, wantError: "unknown candidate ID",
		},
		{
			name: "unknown synthesis citation",
			build: func(acquisition *scriptedAcquisition) (SearchTermsStation, RelevanceStation, GroundedSynthesisStation) {
				acquisition.discoveries["citation"] = discoverOutcome{report: candidateReport("citation", candidate)}
				acquisition.documents[candidate.ID] = documentFixture(candidate.URL, candidate.Title, "short evidence")
				return &recordingTermsStation{}, &recordingRelevanceStation{decision: RelevanceDecision{
					Outcome: RelevanceSelected, CandidateIDs: []websearch.CandidateID{candidate.ID},
				}}, &recordingSynthesisStation{decision: GroundedSynthesisDecision{Paragraphs: []GroundedParagraph{{Text: "Claim.", EvidenceIDs: []EvidenceID{"evidence_unknown"}}}}}
			},
			query: "citation", budget: 2_000, wantError: "unknown evidence ID",
		},
		{
			name: "model authored citation syntax",
			build: func(acquisition *scriptedAcquisition) (SearchTermsStation, RelevanceStation, GroundedSynthesisStation) {
				document := documentFixture(candidate.URL, candidate.Title, "short evidence")
				acquisition.discoveries["citation syntax"] = discoverOutcome{report: candidateReport("citation syntax", candidate)}
				acquisition.documents[candidate.ID] = document
				return &recordingTermsStation{}, &recordingRelevanceStation{decision: RelevanceDecision{
					Outcome: RelevanceSelected, CandidateIDs: []websearch.CandidateID{candidate.ID},
				}}, &recordingSynthesisStation{decision: GroundedSynthesisDecision{Paragraphs: []GroundedParagraph{{Text: "Claim [99].", EvidenceIDs: []EvidenceID{evidenceID(document.ID)}}}}}
			},
			query: "citation syntax", budget: 2_000, wantError: "model-authored citation syntax",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			acquisition := &scriptedAcquisition{discoveries: map[string]discoverOutcome{}, documents: map[websearch.CandidateID]websearch.Document{}}
			terms, relevance, synthesis := test.build(acquisition)
			machine := newFixtureMachine(t, Objective{
				ID: "objective_invalid", Question: "Answer the bounded question.", InitialQuery: test.query,
				Acceptance: exactAcceptance(), Status: ObjectivePending,
			}, acquisition, terms, relevance, synthesis, test.budget)
			if _, err := machine.Run(context.Background()); err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Run error=%v want substring %q", err, test.wantError)
			}
		})
	}
}

func TestCancellationPreventsSynthesisCall(t *testing.T) {
	candidate := candidateFixture("https://cancel.example/doc", "Cancel")
	ctx, cancel := context.WithCancel(context.Background())
	acquisition := &cancelingAcquisition{
		delegate: &scriptedAcquisition{
			discoveries: map[string]discoverOutcome{"cancel": {report: candidateReport("cancel", candidate)}},
			documents:   map[websearch.CandidateID]websearch.Document{candidate.ID: documentFixture(candidate.URL, candidate.Title, "evidence")},
		},
		cancel: cancel,
	}
	synthesis := &recordingSynthesisStation{}
	machine := newFixtureMachine(t, Objective{
		ID: "objective_cancel", Question: "Will this stop?", InitialQuery: "cancel",
		Acceptance: exactAcceptance(), Status: ObjectivePending,
	}, acquisition, &recordingTermsStation{}, &recordingRelevanceStation{}, synthesis, 2_000)
	result, err := machine.Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error=%v want context canceled", err)
	}
	if !reflect.DeepEqual(result, Result{}) {
		t.Fatalf("canceled acquisition returned partial state: %#v", result)
	}
	if synthesis.calls != 0 {
		t.Fatalf("synthesis calls=%d want 0", synthesis.calls)
	}
}

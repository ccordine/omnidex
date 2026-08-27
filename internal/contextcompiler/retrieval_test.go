package contextcompiler

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestFirstTurnWithoutSearchableAuthoritySkipsSearchTerms(t *testing.T) {
	terms := &scriptedTermsStation{decision: assemblyline.ContextSearchTermsDecision{
		Schema: assemblyline.ContextSearchTermsSchemaV1,
		Terms:  []string{"must not be requested"},
	}}
	provider := &scriptedProvider{
		availability: SearchUnavailable,
		set:          CandidateSet{},
	}
	result, err := Compile(t.Context(), Request{ExactInstruction: "Hello."}, provider, Stations{
		Terms: terms,
	})
	if err != nil {
		t.Fatal(err)
	}
	if terms.calls != 0 || provider.availabilityCalls != 1 || provider.calls != 1 ||
		provider.gotTerms == nil || len(provider.gotTerms) != 0 {
		t.Fatalf(
			"terms/availability/retrieval=%d/%d/%d concepts=%#v",
			terms.calls, provider.availabilityCalls, provider.calls, provider.gotTerms,
		)
	}
	if result.ModelCalls != 0 || result.SearchTermsCalls != 0 ||
		len(result.Context.Capsules) != 0 {
		t.Fatalf("first-turn result=%#v", result)
	}
}

func TestUnavailableSearchStillRanksMechanicallyRetainedRecentContext(t *testing.T) {
	recent := candidate(
		t, "conversation_exchange", "CTX_1",
		"user message:\nKeep the brass key.\nassistant response:\nI put it in the blue case.",
	)
	terms := &scriptedTermsStation{decision: assemblyline.ContextSearchTermsDecision{
		Schema: assemblyline.ContextSearchTermsSchemaV1,
		Terms:  []string{"brass key"},
	}}
	relevance := &scriptedRelevanceStation{ids: []string{"CTX_1"}}
	provider := &scriptedProvider{
		availability: SearchUnavailable,
		set: CandidateSet{Optional: []assemblyline.ContextCandidateAuthority{
			recent,
		}},
	}
	result, err := Compile(t.Context(), Request{
		ExactInstruction: "Use it now.",
	}, provider, Stations{Terms: terms, Relevance: relevance})
	if err != nil {
		t.Fatal(err)
	}
	if terms.calls != 0 || relevance.calls != 1 ||
		len(relevance.input.RetrievalConcepts) != 0 ||
		result.SearchTermsCalls != 0 || result.RelevanceCalls != 1 || result.ModelCalls != 1 ||
		len(result.Context.Capsules) != 1 || result.Context.Capsules[0].Content != recent.Content {
		t.Fatalf(
			"terms/relevance=%d/%d input=%#v result=%#v",
			terms.calls, relevance.calls, relevance.input, result,
		)
	}
}

func TestUnknownSearchAvailabilityFailsBeforeSemanticWorkOrRetrieval(t *testing.T) {
	terms := &scriptedTermsStation{}
	provider := &scriptedProvider{availability: SearchAvailability("unknown")}
	_, err := Compile(t.Context(), Request{ExactInstruction: "Recall it."}, provider, Stations{
		Terms: terms,
	})
	if err == nil || !strings.Contains(err.Error(), "search availability") {
		t.Fatalf("error=%v", err)
	}
	if terms.calls != 0 || provider.calls != 0 || provider.availabilityCalls != 1 {
		t.Fatalf(
			"terms/retrieval/availability=%d/%d/%d",
			terms.calls, provider.calls, provider.availabilityCalls,
		)
	}
}

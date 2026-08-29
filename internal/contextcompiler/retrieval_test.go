package contextcompiler

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

func TestFirstTurnWithoutSearchableAuthorityUsesNoRetrievalQuery(t *testing.T) {
	provider := &scriptedProvider{
		availability: SearchUnavailable,
		set:          CandidateSet{},
	}
	result, err := Compile(
		t.Context(), contextRequest("Hello."), provider, Stations{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if provider.availabilityCalls != 1 || provider.calls != 1 ||
		provider.gotTerms == nil || len(provider.gotTerms) != 0 {
		t.Fatalf(
			"availability/retrieval=%d/%d queries=%#v",
			provider.availabilityCalls, provider.calls, provider.gotTerms,
		)
	}
	if result.ModelCalls != 0 || len(result.Context.Capsules) != 0 {
		t.Fatalf("first-turn result=%#v", result)
	}
}

func TestUnavailableSearchStillRanksMechanicallyRetainedRecentContext(t *testing.T) {
	recent := candidate(
		t, "conversation_exchange", "CTX_1",
		"user message:\nKeep the brass key.\nassistant response:\nI put it in the blue case.",
	)
	relevance := &scriptedRelevanceStation{ids: []string{"CTX_1"}}
	provider := &scriptedProvider{
		availability: SearchUnavailable,
		set: CandidateSet{Optional: []assemblyline.ContextCandidateAuthority{
			recent,
		}},
	}
	result, err := Compile(
		t.Context(), contextRequest("Use it now."), provider,
		Stations{Relevance: relevance},
	)
	if err != nil {
		t.Fatal(err)
	}
	if relevance.calls != 1 || result.RelevanceCalls != 1 || result.ModelCalls != 1 ||
		len(result.Context.Capsules) != 1 || result.Context.Capsules[0].Content != recent.Content {
		t.Fatalf("relevance=%d input=%#v result=%#v", relevance.calls, relevance.input, result)
	}
}

func TestAvailableSearchUsesExactInstructionAtFourKiBAuthorityCeiling(t *testing.T) {
	exact := strings.Repeat("x", model.MaxFreeFormTurnBytes)
	provider := &scriptedProvider{availability: SearchAvailable}
	result, err := Compile(t.Context(), contextRequest(exact), provider, Stations{})
	if err != nil {
		t.Fatal(err)
	}
	if result.ModelCalls != 0 || provider.calls != 1 || len(provider.gotTerms) != 1 ||
		provider.gotTerms[0] != exact {
		t.Fatalf("result=%#v provider queries=%#v", result, provider.gotTerms)
	}
}

func TestUnknownSearchAvailabilityFailsBeforeRetrieval(t *testing.T) {
	provider := &scriptedProvider{availability: SearchAvailability("unknown")}
	_, err := Compile(t.Context(), contextRequest("Recall it."), provider, Stations{})
	if err == nil || !strings.Contains(err.Error(), "search availability") {
		t.Fatalf("error=%v", err)
	}
	if provider.calls != 0 || provider.availabilityCalls != 1 {
		t.Fatalf(
			"retrieval/availability=%d/%d", provider.calls, provider.availabilityCalls,
		)
	}
}

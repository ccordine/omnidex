package contextcompiler

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type contextReceiptProvider struct {
	set CandidateSet
}

func (provider *contextReceiptProvider) SearchAvailability(
	context.Context,
) (SearchAvailability, error) {
	return SearchUnavailable, nil
}

func (provider *contextReceiptProvider) Retrieve(
	context.Context,
	[]string,
) (CandidateSet, error) {
	return provider.set, nil
}

type contextReceiptStations struct {
	relevanceReceipt    StationReceipt
	minificationReceipt StationReceipt
	relevanceCalls      int
	minificationCalls   int
}

func (stations *contextReceiptStations) Relate(
	_ context.Context,
	input assemblyline.ContextRelevanceRelationInput,
) (assemblyline.ContextRelevanceRelationResult, StationReceipt, error) {
	stations.relevanceCalls++
	decision, err := assemblyline.DecodeContextRelevanceRelationResult(
		input, assemblyline.ContextCandidateDirectlyRelevant,
	)
	return decision, stations.relevanceReceipt, err
}

func (stations *contextReceiptStations) Minify(
	_ context.Context,
	input assemblyline.ContextMinificationInput,
) (assemblyline.ContextMinificationDecision, StationReceipt, error) {
	stations.minificationCalls++
	decision := assemblyline.ContextMinificationDecision{
		Schema:         assemblyline.ContextMinificationSchemaV1,
		MinimalContext: "The retained authorities jointly establish the current release.",
	}
	return decision, stations.minificationReceipt, decision.ValidateFor(input)
}

func TestContextCompilerPreservesMixedRestoredAndFreshReceipts(t *testing.T) {
	t.Parallel()
	required, err := assemblyline.NewContextCandidateAuthority(
		"conversation_exchange", "CTX_1",
		"Required authority: "+strings.Repeat("a", 1_300),
	)
	if err != nil {
		t.Fatal(err)
	}
	optional, err := assemblyline.NewContextCandidateAuthority(
		"durable_memory", "CTX_2",
		"Optional authority: "+strings.Repeat("b", 1_300),
	)
	if err != nil {
		t.Fatal(err)
	}
	stations := &contextReceiptStations{
		relevanceReceipt:    StationReceipt{Reused: true},
		minificationReceipt: StationReceipt{Calls: 1},
	}
	result, err := Compile(
		t.Context(),
		Request{
			ExactInstruction: "Which release is current?", ModelInstruction: "Which release is current?",
			KnownArtifactPaths: []string{},
			Retrieval:          &RetrievalDirective{Availability: SearchUnavailable},
		},
		&contextReceiptProvider{set: CandidateSet{
			Required: []assemblyline.ContextCandidateAuthority{required},
			Optional: []assemblyline.ContextCandidateAuthority{optional},
		}},
		Stations{Relevance: stations, Minification: stations},
	)
	if err != nil {
		t.Fatal(err)
	}
	if stations.relevanceCalls != 1 || stations.minificationCalls != 1 ||
		result.RelevanceCalls != 0 || result.MinificationCalls != 1 ||
		result.ModelCalls != 1 || len(result.Context.Capsules) != 1 {
		t.Fatalf("stations=%+v result=%+v", stations, result)
	}
}

func TestContextCompilerRejectsZeroCallsWithoutRestoredProof(t *testing.T) {
	t.Parallel()
	optional, err := assemblyline.NewContextCandidateAuthority(
		"conversation_exchange", "CTX_1", "Version 2 is current.",
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		name    string
		receipt StationReceipt
	}{
		{name: "zero without proof"},
		{name: "fresh marked reused", receipt: StationReceipt{Calls: 1, Reused: true}},
	} {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			stations := &contextReceiptStations{relevanceReceipt: fixture.receipt}
			result, err := Compile(
				t.Context(),
				Request{
					ExactInstruction: "Which release is current?", ModelInstruction: "Which release is current?",
					KnownArtifactPaths: []string{},
					Retrieval:          &RetrievalDirective{Availability: SearchUnavailable},
				},
				&contextReceiptProvider{set: CandidateSet{
					Optional: []assemblyline.ContextCandidateAuthority{optional},
				}},
				Stations{Relevance: stations},
			)
			if err == nil || result.ModelCalls != 0 || result.RelevanceCalls != 0 {
				t.Fatalf("receipt=%+v result=%+v error=%v", fixture.receipt, result, err)
			}
		})
	}
}

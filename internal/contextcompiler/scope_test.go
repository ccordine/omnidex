package contextcompiler

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type scopeCapturingStations struct {
	relevance    assemblyline.ContextScope
	minification assemblyline.ContextScope
}

func (station *scopeCapturingStations) SelectRelevant(
	_ context.Context,
	input assemblyline.ContextRelevanceInput,
) (assemblyline.ContextRelevanceDecision, StationReceipt, error) {
	station.relevance = input.Scope
	ids := make([]string, len(input.CandidateAuthorities))
	for index, candidate := range input.CandidateAuthorities {
		ids[index] = candidate.CandidateID
	}
	decision := assemblyline.ContextRelevanceDecision{
		Schema: assemblyline.ContextRelevanceSchemaV1, ReferencedCandidateIDs: ids,
	}
	return decision, StationReceipt{Calls: 1}, decision.ValidateFor(input)
}

func (station *scopeCapturingStations) Minify(
	_ context.Context,
	input assemblyline.ContextMinificationInput,
) (assemblyline.ContextMinificationDecision, StationReceipt, error) {
	station.minification = input.Scope
	decision := assemblyline.ContextMinificationDecision{
		Schema:         assemblyline.ContextMinificationSchemaV1,
		MinimalContext: "The bridge remains raised.",
	}
	return decision, StationReceipt{Calls: 1}, decision.ValidateFor(input)
}

func TestCompilePropagatesRoleplayScopeThroughEveryContextStation(t *testing.T) {
	t.Parallel()
	stations := &scopeCapturingStations{}
	provider := &scriptedProvider{availability: SearchAvailable, set: CandidateSet{
		Optional: []assemblyline.ContextCandidateAuthority{
			candidate(t, "simulation_event", "CTX_1", strings.Repeat("a", 1500)),
			candidate(t, "simulation_event", "CTX_2", strings.Repeat("b", 1500)),
		},
	}}
	result, err := Compile(t.Context(), Request{
		ExactInstruction:   "Continue from the bridge.",
		ModelInstruction:   "Continue from the bridge.",
		KnownArtifactPaths: []string{},
		Scope:              assemblyline.ContextScopeRoleplaySimulation,
	}, provider, Stations{Relevance: stations, Minification: stations})
	if err != nil {
		t.Fatal(err)
	}
	if stations.relevance != assemblyline.ContextScopeRoleplaySimulation ||
		stations.minification != assemblyline.ContextScopeRoleplaySimulation {
		t.Fatalf("station scopes relevance/minification=%q/%q",
			stations.relevance, stations.minification)
	}
	if result.RelevanceCalls != 1 || result.MinificationCalls != 1 {
		t.Fatalf("scoped result=%#v", result)
	}
}

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

func (station *scopeCapturingStations) Relate(
	_ context.Context,
	input assemblyline.ContextRelevanceRelationInput,
) (assemblyline.ContextRelevanceRelationResult, StationReceipt, error) {
	station.relevance = input.Scope
	decision, err := assemblyline.DecodeContextRelevanceRelationResult(
		input, assemblyline.ContextCandidateDirectlyRelevant,
	)
	return decision, StationReceipt{Calls: 1}, err
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
	if result.RelevanceCalls != 2 || result.MinificationCalls != 1 {
		t.Fatalf("scoped result=%#v", result)
	}
}

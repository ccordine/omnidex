package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestCapabilityGraphExposesOnlySelectedDirectDependencies(t *testing.T) {
	t.Parallel()

	requirements := []assemblyline.Requirement{
		{ID: "requirement_001", SourceQuote: "filter visible records"},
		{ID: "requirement_002", SourceQuote: "show a selected record summary"},
		{ID: "requirement_003", SourceQuote: "export the current view"},
	}
	pairs := directCodingCapabilityPairs("browser inventory", requirements)
	results := []directCodingCapabilityResult{
		{Pair: pairs[0], Decision: assemblyline.CapabilityRelationDecision{
			Schema: assemblyline.CapabilityRelationSchemaV1, Relation: assemblyline.CapabilityRightReadsLeft,
		}},
		{Pair: pairs[1], Decision: assemblyline.CapabilityRelationDecision{
			Schema: assemblyline.CapabilityRelationSchemaV1, Relation: assemblyline.CapabilityIndependent,
		}},
		{Pair: pairs[2], Decision: assemblyline.CapabilityRelationDecision{
			Schema: assemblyline.CapabilityRelationSchemaV1, Relation: assemblyline.CapabilityRightReadsLeft,
		}},
	}
	graph, err := assembleDirectCodingCapabilityGraph(requirements, results)
	if err != nil {
		t.Fatal(err)
	}
	if len(graph["requirement_001"]) != 0 || len(graph["requirement_002"]) != 1 ||
		len(graph["requirement_003"]) != 1 {
		t.Fatalf("unexpected capability graph: %#v", graph)
	}
	if graph["requirement_002"][0].CapabilityID != "capability_001" ||
		graph["requirement_003"][0].CapabilityID != "capability_002" {
		t.Fatalf("capability identities are not code-owned: %#v", graph)
	}
}

func TestCapabilityGraphRejectsUnselectedOrUnorderedBindings(t *testing.T) {
	t.Parallel()

	requirements := []assemblyline.Requirement{
		{ID: "requirement_001", SourceQuote: "filter visible records"},
		{ID: "requirement_002", SourceQuote: "show a selected record summary"},
	}
	graph := directCodingCapabilityGraph{
		"requirement_001": {{
			RequirementID: "requirement_002", CapabilityID: "capability_999", Purpose: "invented",
		}},
		"requirement_002": nil,
	}
	if err := validateDirectCodingCapabilityGraph(requirements, graph); err == nil ||
		!strings.Contains(err.Error(), "code-owned") {
		t.Fatalf("invalid graph error=%v", err)
	}
}

func TestCapabilityGraphRejectsWorkloadsBeyondItsBoundedAssembly(t *testing.T) {
	t.Parallel()

	requirements := make([]assemblyline.Requirement, maxDirectCodingRequirements+1)
	if err := validateDirectCodingRequirementCount(requirements); err == nil ||
		!strings.Contains(err.Error(), "received") {
		t.Fatalf("oversized requirement error=%v", err)
	}
}

func TestCapabilityGraphMapsEveryRegisteredDirectionWithoutInversion(t *testing.T) {
	t.Parallel()
	requirements := []assemblyline.Requirement{
		{ID: "requirement_001", SourceQuote: "maintain the selected document"},
		{ID: "requirement_002", SourceQuote: "show a summary of the selected document"},
	}
	pair := directCodingCapabilityPairs("document workspace", requirements)[0]
	fixtures := []struct {
		relation  assemblyline.CapabilityRelation
		leftDeps  int
		rightDeps int
	}{
		{relation: assemblyline.CapabilityIndependent},
		{relation: assemblyline.CapabilityLeftReadsRight, leftDeps: 1},
		{relation: assemblyline.CapabilityRightReadsLeft, rightDeps: 1},
	}
	for _, fixture := range fixtures {
		graph, err := assembleDirectCodingCapabilityGraph(
			requirements,
			[]directCodingCapabilityResult{{
				Pair: pair,
				Decision: assemblyline.CapabilityRelationDecision{
					Schema:   assemblyline.CapabilityRelationSchemaV1,
					Relation: fixture.relation,
				},
			}},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(graph[requirements[0].ID]) != fixture.leftDeps ||
			len(graph[requirements[1].ID]) != fixture.rightDeps {
			t.Fatalf("relation=%s graph=%#v", fixture.relation, graph)
		}
	}
}

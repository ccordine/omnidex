package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const liveApplicationResultRelationBoundaryScope = "live-application-result-relation-boundary-v1"

func TestLiveApplicationResultRelationBoundaryQualification(t *testing.T) {
	ctx, modelName, transport := newLiveApplicationSemanticBoundaryTransport(
		t,
		liveApplicationResultRelationBoundaryScope,
	)
	fixtures := []struct {
		name, candidate, relation string
	}{
		{
			name:      "text-transformation",
			candidate: "Display submitted text converted to Unicode lowercase.",
			relation:  assemblyline.ApplicationRequirementExplicitResultRelation,
		},
		{
			name:      "numeric-ordering",
			candidate: "Display submitted numbers ordered in ascending numeric order.",
			relation:  assemblyline.ApplicationRequirementExplicitResultRelation,
		},
		{
			name:      "document-recorded-key",
			candidate: "Display uploaded documents grouped by their recorded owner identifier.",
			relation:  assemblyline.ApplicationRequirementExplicitResultRelation,
		},
		{
			name:      "document-unstated-key",
			candidate: "Display uploaded documents in the most useful groups.",
			relation:  assemblyline.ApplicationRequirementMissingResultRelation,
		},
		{
			name:      "telemetry-warning",
			candidate: "Show a prominent warning when a telemetry packet is invalid.",
			relation:  assemblyline.ApplicationRequirementNoDerivedResult,
		},
		{
			name:      "telemetry-selection",
			candidate: "Show the best recovery route when a telemetry packet is invalid.",
			relation:  assemblyline.ApplicationRequirementMissingResultRelation,
		},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			input := liveApplicationResultRelationInput(t, fixture.candidate)
			job, err := assemblyline.NewApplicationRequirementCandidateResultRelationJob(input)
			if err != nil {
				t.Fatal(err)
			}
			start := transport.callCount()
			raw, err := transport.execute(ctx, job, modelName)
			if err != nil {
				t.Fatal(err)
			}
			result, err := assemblyline.DecodeApplicationRequirementCandidateResultRelationResult(
				input,
				raw.Candidate,
			)
			if err != nil {
				t.Fatal(err)
			}
			if result.Relation != fixture.relation {
				t.Fatalf("relation=%q, want %q", result.Relation, fixture.relation)
			}
			calls := transport.callsFrom(start)
			if len(calls) != 1 ||
				calls[0].kind != assemblyline.WorkApplicationRequirementCandidateResultRelation {
				t.Fatalf("unexpected exact station calls: %+v", calls)
			}
			logLiveCodingQualification(t, fixture.name, modelName, "not-applicable", calls)
		})
	}
}

func liveApplicationResultRelationInput(
	t *testing.T,
	candidate string,
) assemblyline.ApplicationRequirementCandidateResultRelationInput {
	t.Helper()
	kindInput := assemblyline.ApplicationRequirementCandidateKindInput{Candidate: candidate}
	kind, err := assemblyline.DecodeApplicationRequirementCandidateKindResult(
		kindInput,
		assemblyline.ApplicationRequirementCandidateTaskLocal,
	)
	if err != nil {
		t.Fatal(err)
	}
	cardinalityInput := assemblyline.ApplicationRequirementCandidateCardinalityInput{
		Candidate: candidate,
	}
	cardinality, err := assemblyline.DecodeApplicationRequirementCandidateCardinalityResult(
		cardinalityInput,
		assemblyline.ApplicationRequirementOneRuntimeOutcome,
	)
	if err != nil {
		t.Fatal(err)
	}
	return assemblyline.ApplicationRequirementCandidateResultRelationInput{
		Candidate:   candidate,
		Kind:        kind,
		Cardinality: cardinality,
	}
}

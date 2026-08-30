package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const liveApplicationOutcomeRelationBoundaryScope = "live-application-outcome-relation-boundary-v1"

func TestLiveApplicationOutcomeRelationBoundaryQualification(t *testing.T) {
	ctx, modelName, transport := newLiveApplicationSemanticBoundaryTransport(
		t,
		liveApplicationOutcomeRelationBoundaryScope,
	)
	fixtures := []struct {
		name, candidate, accepted, relation string
	}{
		{
			name:      "calculation-accuracy-is-same-result",
			candidate: "The application produces a numeric result that is mathematically accurate for the calculation performed by the user.",
			accepted:  "The application displays a numeric result for the calculation performed by the user.",
			relation:  assemblyline.ApplicationRequirementSameRuntimeOutcome,
		},
		{
			name:      "transformation-exactness-is-same-result",
			candidate: "Display the exact Unicode-lowercase value produced from the submitted text.",
			accepted:  "Display submitted text converted to Unicode lowercase.",
			relation:  assemblyline.ApplicationRequirementSameRuntimeOutcome,
		},
		{
			name:      "filter-conformance-is-same-result",
			candidate: "Display every record selected by evaluating the filter expression entered by the user.",
			accepted:  "Display records selected by evaluating the filter expression entered by the user.",
			relation:  assemblyline.ApplicationRequirementSameRuntimeOutcome,
		},
		{
			name:      "different-transformation-rule-is-distinct",
			candidate: "Display the exact Unicode-uppercase value produced from the submitted text.",
			accepted:  "Display submitted text converted to Unicode lowercase.",
			relation:  assemblyline.ApplicationRequirementDistinctRuntimeOutcomes,
		},
		{
			name:      "persistence-is-distinct",
			candidate: "Preserve the displayed lowercase result after a page reload.",
			accepted:  "Display submitted text converted to Unicode lowercase.",
			relation:  assemblyline.ApplicationRequirementDistinctRuntimeOutcomes,
		},
		{
			name:      "latency-is-distinct",
			candidate: "Display the lowercase result within 100 milliseconds of submission.",
			accepted:  "Display submitted text converted to Unicode lowercase.",
			relation:  assemblyline.ApplicationRequirementDistinctRuntimeOutcomes,
		},
		{
			name:      "accessibility-response-is-distinct",
			candidate: "Announce the lowercase result to assistive technology.",
			accepted:  "Display submitted text converted to Unicode lowercase.",
			relation:  assemblyline.ApplicationRequirementDistinctRuntimeOutcomes,
		},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			candidateAuthority := liveApplicationResultRelationInput(t, fixture.candidate)
			acceptedAuthority := liveApplicationResultRelationInput(t, fixture.accepted)
			acceptedRelation, err := assemblyline.DecodeApplicationRequirementCandidateResultRelationResult(
				acceptedAuthority,
				assemblyline.ApplicationRequirementExplicitResultRelation,
			)
			if err != nil {
				t.Fatal(err)
			}
			input := assemblyline.ApplicationRequirementCandidateOutcomeRelationInput{
				Candidate:              fixture.candidate,
				Kind:                   candidateAuthority.Kind,
				Cardinality:            candidateAuthority.Cardinality,
				AcceptedRequirement:    fixture.accepted,
				AcceptedResultRelation: acceptedRelation,
			}
			job, err := assemblyline.NewApplicationRequirementCandidateOutcomeRelationJob(input)
			if err != nil {
				t.Fatal(err)
			}
			start := transport.callCount()
			raw, err := transport.execute(ctx, job, modelName)
			if err != nil {
				t.Fatal(err)
			}
			result, err := assemblyline.DecodeApplicationRequirementCandidateOutcomeRelationResult(
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
				calls[0].kind != assemblyline.WorkApplicationRequirementCandidateOutcomeRelation {
				t.Fatalf("unexpected exact station calls: %+v", calls)
			}
			logLiveCodingQualification(t, fixture.name, modelName, "not-applicable", calls)
		})
	}
}

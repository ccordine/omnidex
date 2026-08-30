package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const liveApplicationResponseMeaningScope = "live-application-response-meaning-qualification-v1"

func TestLiveApplicationResponseMeaningQualification(t *testing.T) {
	ctx, modelName, transport := newLiveApplicationSemanticBoundaryTransport(
		t,
		liveApplicationResponseMeaningScope,
	)

	fixtures := []struct {
		name, request, candidate, accepted string
	}{
		{
			name:      "form-behavior",
			request:   "Build a browser submission form that shows a validation summary and disables the submit control when submission is invalid.",
			candidate: "Show a validation summary and disable the submit control when submission is invalid.",
			accepted:  "Show a validation summary when submission is invalid.",
		},
		{
			name:      "session-behavior",
			request:   "Build a session monitor that records an audit entry and revokes the access token when a session expires.",
			candidate: "Record an audit entry and revoke the access token when a session expires.",
			accepted:  "Record an audit entry when a session expires.",
		},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			start := transport.callCount()
			cardinalityInput := assemblyline.ApplicationRequirementCandidateCardinalityInput{
				Candidate: fixture.candidate,
			}
			cardinalityJob, err := assemblyline.NewApplicationRequirementCandidateCardinalityJob(
				cardinalityInput,
			)
			if err != nil {
				t.Fatal(err)
			}
			cardinalityRaw, err := transport.execute(ctx, cardinalityJob, modelName)
			if err != nil {
				t.Fatal(err)
			}
			cardinality, err := assemblyline.DecodeApplicationRequirementCandidateCardinalityResult(
				cardinalityInput,
				cardinalityRaw.Candidate,
			)
			if err != nil {
				t.Fatal(err)
			}
			if cardinality.Relation != assemblyline.ApplicationRequirementMultipleRuntimeOutcomes {
				t.Fatalf("cardinality=%q, want %q", cardinality.Relation, assemblyline.ApplicationRequirementMultipleRuntimeOutcomes)
			}

			applicationContext, err := assemblyline.BootstrapApplicationContext(
				fixture.request,
				assemblyline.ApplicationWorkspaceEmpty,
			)
			if err != nil {
				t.Fatal(err)
			}
			coverageInput := assemblyline.ApplicationRequirementCoverageInput{
				UserRequest:          fixture.request,
				Context:              applicationContext,
				AcceptedRequirements: []string{fixture.accepted},
				ExcludedCandidates:   []string{},
				ZeroDeltas:           []assemblyline.ApplicationRequirementCandidateZeroDelta{},
			}
			coverageJob, err := assemblyline.NewApplicationRequirementCoverageJob(coverageInput)
			if err != nil {
				t.Fatal(err)
			}
			coverageRaw, err := transport.execute(ctx, coverageJob, modelName)
			if err != nil {
				t.Fatal(err)
			}
			coverage, err := assemblyline.DecodeApplicationRequirementCoverageLeaf(
				coverageInput,
				coverageRaw.Candidate,
			)
			if err != nil {
				t.Fatal(err)
			}
			if coverage.Relation != assemblyline.ApplicationRequirementRemains {
				t.Fatalf("coverage=%q, want %q", coverage.Relation, assemblyline.ApplicationRequirementRemains)
			}
			calls := transport.callsFrom(start)
			if len(calls) != 2 ||
				calls[0].kind != assemblyline.WorkApplicationRequirementCandidateCardinality ||
				calls[1].kind != assemblyline.WorkApplicationRequirementCoverage {
				t.Fatalf("unexpected exact station calls: %+v", calls)
			}
			logLiveCodingQualification(t, fixture.name, modelName, "not-applicable", calls)
		})
	}
}

package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestLiveCodingQualificationResultRelationGroundingProjectionIsBound(t *testing.T) {
	t.Parallel()
	const request = "Build a browser unit converter that converts yards to feet using three feet per yard."
	const candidate = "Accept a yard value and display the correct converted value."
	candidateAuthority := applicationRequirementCandidateResultRelationAuthorityForTest(
		t,
		candidate,
	)
	missing := applicationRequirementCandidateResultRelationReceiptForTest(
		t,
		candidateAuthority,
		assemblyline.ApplicationRequirementMissingResultRelation,
	)
	context, err := assemblyline.BootstrapApplicationContext(
		request,
		assemblyline.ApplicationWorkspaceEmpty,
	)
	if err != nil {
		t.Fatal(err)
	}
	job, err := assemblyline.NewApplicationRequirementCandidateResultRelationGroundingJob(
		assemblyline.ApplicationRequirementCandidateResultRelationGroundingInput{
			ImmutableRequest: request, CandidateAuthority: candidateAuthority,
			Context:               context,
			MissingResultRelation: missing,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	testCase := liveCodingQualificationCase{name: "grounding-projection", request: request}
	if err := validateLiveCodingQualificationProjection(testCase, job, prompt); err != nil {
		t.Fatal(err)
	}
	testCase.request = "Build another application."
	if err := validateLiveCodingQualificationProjection(testCase, job, prompt); err == nil {
		t.Fatal("grounding projection accepted a different immutable request")
	}
}

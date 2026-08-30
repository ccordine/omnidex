package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const liveApplicationCandidateAuthorizationBoundaryScope = "live-application-candidate-authorization-boundary-v3"

func TestLiveApplicationRequirementCandidateAuthorizationBoundaryQualification(t *testing.T) {
	ctx, modelName, transport := newLiveApplicationSemanticBoundaryTransport(
		t,
		liveApplicationCandidateAuthorizationBoundaryScope,
	)
	fixtures := []struct {
		name, request, candidate, relation string
	}{
		{
			name:      "imperative-grammatical-normalization",
			request:   "Resize the submitted image.",
			candidate: "The finished software resizes the submitted image.",
			relation:  assemblyline.ApplicationRequirementCandidateEntailed,
		},
		{
			name:      "unrelated-imperative-grammatical-normalization",
			request:   "Archive each received message.",
			candidate: "The finished software archives each received message.",
			relation:  assemblyline.ApplicationRequirementCandidateEntailed,
		},
		{
			name:      "purpose-name-core-action",
			request:   "Build an image resizer in Rust.",
			candidate: "The finished software resizes images.",
			relation:  assemblyline.ApplicationRequirementCandidateEntailed,
		},
		{
			name:      "unrelated-purpose-name-core-action",
			request:   "Create a barcode scanner for a mobile browser.",
			candidate: "The finished software scans barcodes.",
			relation:  assemblyline.ApplicationRequirementCandidateEntailed,
		},
		{
			name:      "purpose-name-added-mechanism",
			request:   "Create a barcode scanner for a mobile browser.",
			candidate: "The finished software scans barcodes through the device camera.",
			relation:  assemblyline.ApplicationRequirementCandidateNotEntailed,
		},
		{
			name:      "purpose-name-added-customary-feature",
			request:   "Build an image resizer in Rust.",
			candidate: "The finished software resizes images into several output formats.",
			relation:  assemblyline.ApplicationRequirementCandidateNotEntailed,
		},
		{
			name:      "framework-only-added-runtime",
			request:   "Use React.",
			candidate: "The finished software renders a user interface using React.",
			relation:  assemblyline.ApplicationRequirementCandidateNotEntailed,
		},
		{
			name:      "database-only-added-runtime",
			request:   "Use PostgreSQL.",
			candidate: "The finished software stores user records persistently.",
			relation:  assemblyline.ApplicationRequirementCandidateNotEntailed,
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			applicationContext, err := assemblyline.BootstrapApplicationContext(
				fixture.request,
				assemblyline.ApplicationWorkspaceEmpty,
			)
			if err != nil {
				t.Fatal(err)
			}
			input := assemblyline.ApplicationRequirementCandidateAuthorizationInput{
				UserRequest: fixture.request,
				Context:     applicationContext,
				Candidate:   fixture.candidate,
			}
			job, err := assemblyline.NewApplicationRequirementCandidateAuthorizationJob(input)
			if err != nil {
				t.Fatal(err)
			}
			prompt, err := assemblyline.RenderPortableJob(job)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateLiveCodingQualificationProjection(
				liveCodingQualificationCase{name: fixture.name, request: fixture.request},
				job,
				prompt,
			); err != nil {
				t.Fatal(err)
			}

			start := transport.callCount()
			portableResult, err := transport.execute(ctx, job, modelName)
			if err != nil {
				t.Fatal(err)
			}
			result, err := assemblyline.DecodeApplicationRequirementCandidateAuthorizationResult(
				input,
				portableResult.Candidate,
			)
			calls := transport.callsFrom(start)
			logLiveCodingQualification(t, fixture.name, modelName, "not-applicable", calls)
			if err != nil {
				t.Fatal(err)
			}
			if result.Relation != fixture.relation || len(calls) != 1 ||
				calls[0].kind != assemblyline.WorkApplicationRequirementCandidateAuthorization {
				t.Fatalf(
					"authorization=%q calls=%+v want=%q",
					result.Relation,
					calls,
					fixture.relation,
				)
			}
			if calls[0].subject != fixture.candidate {
				t.Fatalf(
					"authorization subject=%q want=%q",
					calls[0].subject,
					fixture.candidate,
				)
			}
		})
	}
}

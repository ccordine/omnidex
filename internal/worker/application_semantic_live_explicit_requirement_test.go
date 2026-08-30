package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const liveApplicationExplicitRequirementScope = "live-application-explicit-requirement-qualification-v1"

func TestLiveApplicationExplicitRequirementQualification(t *testing.T) {
	ctx, modelName, transport := newLiveApplicationSemanticBoundaryTransport(
		t,
		liveApplicationExplicitRequirementScope,
	)
	fixtures := []struct {
		name, request string
		accepted      []string
		relation      string
	}{
		{
			name:    "photo-editor",
			request: "Build a browser photo editor with crop and rotate controls.",
			accepted: []string{
				"Provide a crop control.",
				"Provide a rotate control.",
			},
			relation: assemblyline.ApplicationNoUncoveredRequirement,
		},
		{
			name:    "archive-inspector",
			request: "Build a command-line archive inspector that lists entry names and reports the total entry count.",
			accepted: []string{
				"Print the archive entry names.",
				"Report the total archive entry count.",
			},
			relation: assemblyline.ApplicationNoUncoveredRequirement,
		},
		{
			name:    "import-validator-response",
			request: "Build a command-line import validator. It must print the accepted record count, show clear help, and return a useful error for invalid input. Use Rust, include focused tests, and produce a production build.",
			accepted: []string{
				"Print the accepted record count.",
				"Show clear help.",
			},
			relation: assemblyline.ApplicationRequirementRemains,
		},
		{
			name:    "reconciliation-action",
			request: "Build a browser reconciliation console that stores matched records, reports completion, and reliably quarantines unmatched records.",
			accepted: []string{
				"Store matched records.",
				"Report completion.",
			},
			relation: assemblyline.ApplicationRequirementRemains,
		},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			applicationContext, err := assemblyline.BootstrapApplicationContext(
				fixture.request,
				assemblyline.ApplicationWorkspaceEmpty,
			)
			if err != nil {
				t.Fatal(err)
			}
			input := assemblyline.ApplicationRequirementCoverageInput{
				UserRequest:          fixture.request,
				Context:              applicationContext,
				AcceptedRequirements: append([]string(nil), fixture.accepted...),
				ExcludedCandidates:   []string{},
				ZeroDeltas:           []assemblyline.ApplicationRequirementCandidateZeroDelta{},
			}
			job, err := assemblyline.NewApplicationRequirementCoverageJob(input)
			if err != nil {
				t.Fatal(err)
			}
			start := transport.callCount()
			raw, err := transport.execute(ctx, job, modelName)
			if err != nil {
				t.Fatal(err)
			}
			coverage, err := assemblyline.DecodeApplicationRequirementCoverageLeaf(
				input,
				raw.Candidate,
			)
			if err != nil {
				t.Fatal(err)
			}
			calls := transport.callsFrom(start)
			if len(calls) != 1 || calls[0].kind != assemblyline.WorkApplicationRequirementCoverage {
				t.Fatalf("unexpected exact station calls: %+v", calls)
			}
			logLiveCodingQualification(t, fixture.name, modelName, "not-applicable", calls)
			if coverage.Relation != fixture.relation {
				t.Fatalf("coverage=%q, want %q", coverage.Relation, fixture.relation)
			}
		})
	}
}

package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func TestApplicationServiceEndpointRequirementHasOneExactStationOwner(t *testing.T) {
	t.Parallel()
	job, err := assemblyline.NewApplicationServiceEndpointRequirementJob(
		assemblyline.ApplicationServiceEndpointRequirementInput{
			ProductContext:              "inventory service",
			RequirementQuote:            "Records are normalized before storage.",
			HasDirectCapabilityConsumer: true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	got, err := StationForPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if got != station.CodingServiceEndpointRequirement {
		t.Fatalf("station=%q want=%q", got, station.CodingServiceEndpointRequirement)
	}
}

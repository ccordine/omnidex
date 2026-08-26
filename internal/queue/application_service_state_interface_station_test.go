package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func TestApplicationServiceStateInterfaceHasOneExactStationOwner(t *testing.T) {
	t.Parallel()
	job, err := assemblyline.NewApplicationServiceStateInterfaceJob(
		assemblyline.ApplicationServiceStateInterfaceInput{
			ProductContext: "shipment registry",
			Needs: []assemblyline.ApplicationServiceStateInterfaceNeed{{
				RequirementQuote:   "Store a shipment measurement for later retrieval.",
				Objective:          "Preserve shipment measurements between requests.",
				RequiredBehaviors:  []string{"Retain each measurement with its stable identifier."},
				AcceptanceCriteria: []string{"A later lookup returns the recorded measurement."},
			}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	got, err := StationForPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if got != station.CodingServiceStateInterface {
		t.Fatalf("station=%q want=%q", got, station.CodingServiceStateInterface)
	}
}

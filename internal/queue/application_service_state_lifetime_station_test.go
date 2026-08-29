package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func TestApplicationServiceStateLifetimeHasOneExactStationOwner(t *testing.T) {
	t.Parallel()
	job, err := assemblyline.NewApplicationServiceStateLifetimeJob(
		assemblyline.ApplicationServiceStateLifetimeInput{
			ProductContext:   "inventory service",
			RequirementQuote: "A later request can observe an earlier value.",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	got, err := StationForPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if got != station.CodingServiceStateLifetime {
		t.Fatalf("station=%q want=%q", got, station.CodingServiceStateLifetime)
	}
}

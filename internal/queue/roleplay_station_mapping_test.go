package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func TestRoleplayCanonExtractionHasOneExactStationOwner(t *testing.T) {
	for _, kind := range []assemblyline.WorkKind{
		assemblyline.WorkRoleplayCanonFactCoverage,
		assemblyline.WorkRoleplayCanonFact,
	} {
		got, err := stationForPortableWorkKind(kind)
		if err != nil {
			t.Fatal(err)
		}
		if got != station.RoleplayCanonExtraction {
			t.Fatalf("kind=%q station=%q", kind, got)
		}
	}
}

package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func TestRoleplayCanonExtractionHasOneExactStationOwner(t *testing.T) {
	got, err := stationForPortableWorkKind(assemblyline.WorkRoleplayCanonExtraction)
	if err != nil {
		t.Fatal(err)
	}
	if got != station.RoleplayCanonExtraction {
		t.Fatalf("station=%q", got)
	}
}

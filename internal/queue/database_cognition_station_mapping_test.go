package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func TestDatabaseCognitionPortableWorkHasOneExactStation(t *testing.T) {
	wants := map[assemblyline.WorkKind]station.ID{
		assemblyline.WorkDatabaseSchemaSelection:   station.DatabaseSchemaSelection,
		assemblyline.WorkDatabaseQueryIntent:       station.DatabaseQueryIntent,
		assemblyline.WorkDatabaseEvidenceGap:       station.DatabaseEvidenceGap,
		assemblyline.WorkDatabaseJoinPathSelection: station.DatabaseJoinPathSelection,
	}
	for kind, want := range wants {
		got, err := stationForPortableWorkKind(kind)
		if err != nil {
			t.Fatalf("station for %s: %v", kind, err)
		}
		if got != want {
			t.Fatalf("station for %s=%s want %s", kind, got, want)
		}
	}
}

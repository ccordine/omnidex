package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func TestRepositoryEvidenceRelevancePortableWorkHasOneExactStationOwner(t *testing.T) {
	t.Parallel()
	for kind, want := range map[assemblyline.WorkKind]station.ID{
		assemblyline.WorkRepositoryEvidenceRelevanceLeaf: station.RepositoryEvidenceRelevance,
	} {
		if got, err := stationForPortableWorkKind(kind); err != nil || got != want {
			t.Fatalf("work=%q station=%q error=%v want=%q", kind, got, err, want)
		}
	}
}

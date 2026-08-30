package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func TestRepositoryRequirementInventoryWorkHasOneExactStationOwner(t *testing.T) {
	t.Parallel()
	for _, kind := range []assemblyline.WorkKind{
		assemblyline.WorkRepositoryRequirementInventory,
		assemblyline.WorkRepositoryRequirementCandidateAuthorization,
		assemblyline.WorkRepositoryRequirementCandidateRelation,
	} {
		got, err := stationForPortableWorkKind(kind)
		if err != nil || got != station.CodingRequirements {
			t.Fatalf("work=%q station=%q error=%v", kind, got, err)
		}
	}
}

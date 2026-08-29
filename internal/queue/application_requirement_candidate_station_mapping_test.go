package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func TestApplicationRequirementCandidateWorkHasOneExactStationOwner(t *testing.T) {
	t.Parallel()
	for _, kind := range []assemblyline.WorkKind{
		assemblyline.WorkApplicationRequirementCandidateCardinality,
		assemblyline.WorkApplicationRequirementCandidateKind,
		assemblyline.WorkApplicationRequirementCandidateSplit,
		assemblyline.WorkApplicationRequirementCandidateSplitCorrection,
		assemblyline.WorkApplicationRequirementCandidateDuplicateReplacement,
	} {
		got, err := stationForPortableWorkKind(kind)
		if err != nil || got != station.CodingRequirements {
			t.Fatalf(
				"work=%q station=%q error=%v want=%q",
				kind, got, err, station.CodingRequirements,
			)
		}
	}
}

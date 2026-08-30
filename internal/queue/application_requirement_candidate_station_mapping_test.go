package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func TestApplicationRequirementCandidateWorkHasOneExactStationOwner(t *testing.T) {
	t.Parallel()
	for _, kind := range []assemblyline.WorkKind{
		assemblyline.WorkApplicationRequirementInventory,
		assemblyline.WorkApplicationRequirementCandidateCardinality,
		assemblyline.WorkApplicationRequirementCandidateKind,
		assemblyline.WorkApplicationRequirementCandidateAuthorization,
		assemblyline.WorkApplicationRequirementCandidateOutcomeRelation,
		assemblyline.WorkApplicationRequirementCandidateResultRelation,
		assemblyline.WorkApplicationRequirementCandidateResultRelationGrounding,
		assemblyline.WorkApplicationRequirementCandidateResultRelationCorrection,
		assemblyline.WorkApplicationRequirementCandidatePartition,
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

func TestApplicationRequirementCandidateContentPresenceDimensionsHaveOneExactStationOwner(
	t *testing.T,
) {
	t.Parallel()
	for _, dimension := range []assemblyline.ApplicationRequirementCandidateContentDimension{
		assemblyline.ApplicationRequirementCandidateRuntimeContentDimension,
		assemblyline.ApplicationRequirementCandidateNonRuntimeContentDimension,
	} {
		job, err := assemblyline.NewApplicationRequirementCandidateContentPresenceJob(
			assemblyline.ApplicationRequirementCandidateContentPresenceInput{
				Candidate: "Display the current status.",
				Dimension: dimension,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		got, err := StationForPortableJob(job)
		if err != nil || got != station.CodingRequirements {
			t.Fatalf(
				"dimension=%q station=%q error=%v want=%q",
				dimension,
				got,
				err,
				station.CodingRequirements,
			)
		}
	}
}

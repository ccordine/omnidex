package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func TestApplicationServiceStateLeavesHaveDistinctExactStationOwners(t *testing.T) {
	t.Parallel()
	authority := assemblyline.ApplicationServiceStateInterfaceInput{
		ProductContext: "shipment registry",
		Needs: []assemblyline.ApplicationServiceStateInterfaceNeed{{
			RequirementQuote: "Store a shipment measurement for later retrieval.",
		}},
	}
	parentPurpose := "The stored shipment measurements."
	tests := []struct {
		construct func() (assemblyline.PortableJob, error)
		want      station.ID
	}{
		{func() (assemblyline.PortableJob, error) {
			return assemblyline.NewApplicationStateFieldPurposeInventoryJob(
				assemblyline.ApplicationStateFieldPurposeInventoryInput{Authority: authority},
			)
		}, station.CodingApplicationStateFieldPurposeInventory},
		{func() (assemblyline.PortableJob, error) {
			return assemblyline.NewApplicationStateFieldKindJob(
				assemblyline.ApplicationStateFieldKindInput{
					Authority: authority, FocusedPurpose: parentPurpose,
				},
			)
		}, station.CodingApplicationStateFieldKind},
		{func() (assemblyline.PortableJob, error) {
			return assemblyline.NewApplicationRecordFieldPurposeInventoryJob(
				assemblyline.ApplicationRecordFieldPurposeInventoryInput{
					Authority: authority, ParentPurpose: parentPurpose,
				},
			)
		}, station.CodingApplicationRecordFieldPurposeInventory},
		{func() (assemblyline.PortableJob, error) {
			return assemblyline.NewApplicationRecordFieldKindJob(
				assemblyline.ApplicationRecordFieldKindInput{
					Authority: authority, ParentPurpose: parentPurpose,
					FocusedPurpose: "The shipment measurement label.",
				},
			)
		}, station.CodingApplicationRecordFieldKind},
		{func() (assemblyline.PortableJob, error) {
			return assemblyline.NewApplicationServiceStatePurposeNecessityJob(
				assemblyline.ApplicationServiceStatePurposeNecessityInput{
					Scope:     assemblyline.ApplicationServiceStateRootPurposeScope,
					Authority: authority, CandidatePurpose: parentPurpose,
				},
			)
		}, station.CodingApplicationServiceStatePurposeNecessity},
		{func() (assemblyline.PortableJob, error) {
			return assemblyline.NewApplicationServiceStatePurposeRelationJob(
				assemblyline.ApplicationServiceStatePurposeRelationInput{
					Scope:            assemblyline.ApplicationServiceStateRootPurposeScope,
					CandidatePurpose: "The stored shipment entries.",
					AcceptedPurpose:  parentPurpose,
				},
			)
		}, station.CodingApplicationServiceStatePurposeRelation},
	}
	for _, testCase := range tests {
		job, err := testCase.construct()
		if err != nil {
			t.Fatal(err)
		}
		got, err := StationForPortableJob(job)
		if err != nil {
			t.Fatal(err)
		}
		if got != testCase.want {
			t.Fatalf("kind=%q station=%q want=%q", job.Kind, got, testCase.want)
		}
	}
}

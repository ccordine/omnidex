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
	stateLeaf := assemblyline.ApplicationStateFieldLeafInput{
		Authority: authority, AcceptedFields: []assemblyline.ApplicationServiceStateField{},
	}
	recordLeaf := assemblyline.ApplicationRecordFieldLeafInput{
		Authority: authority, ParentPurpose: "The stored shipment measurements.",
		AcceptedRecordFields: []assemblyline.ApplicationServiceStateRecordField{},
	}
	tests := []struct {
		construct func() (assemblyline.PortableJob, error)
		want      station.ID
	}{
		{func() (assemblyline.PortableJob, error) {
			return assemblyline.NewApplicationStateFieldCoverageJob(stateLeaf)
		}, station.CodingApplicationStateFieldCoverage},
		{func() (assemblyline.PortableJob, error) {
			return assemblyline.NewApplicationStateFieldPurposeJob(stateLeaf)
		}, station.CodingApplicationStateFieldPurpose},
		{func() (assemblyline.PortableJob, error) {
			return assemblyline.NewApplicationStateFieldKindJob(
				assemblyline.ApplicationStateFieldKindInput{
					Authority: authority, AcceptedFields: []assemblyline.ApplicationServiceStateField{},
					FocusedPurpose: "The stored shipment measurements.",
				},
			)
		}, station.CodingApplicationStateFieldKind},
		{func() (assemblyline.PortableJob, error) {
			return assemblyline.NewApplicationRecordFieldCoverageJob(recordLeaf)
		}, station.CodingApplicationRecordFieldCoverage},
		{func() (assemblyline.PortableJob, error) {
			return assemblyline.NewApplicationRecordFieldPurposeJob(recordLeaf)
		}, station.CodingApplicationRecordFieldPurpose},
		{func() (assemblyline.PortableJob, error) {
			return assemblyline.NewApplicationRecordFieldKindJob(
				assemblyline.ApplicationRecordFieldKindInput{
					Authority: authority, ParentPurpose: recordLeaf.ParentPurpose,
					AcceptedRecordFields: []assemblyline.ApplicationServiceStateRecordField{},
					FocusedPurpose:       "The shipment measurement label.",
				},
			)
		}, station.CodingApplicationRecordFieldKind},
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

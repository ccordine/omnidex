package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func TestApplicationServiceStateInterfaceHasOneExactStationOwner(t *testing.T) {
	t.Parallel()
	authority := assemblyline.ApplicationServiceStateInterfaceInput{
		ProductContext: "shipment registry",
		Needs: []assemblyline.ApplicationServiceStateInterfaceNeed{{
			RequirementQuote:  "Store a shipment measurement for later retrieval.",
			Objective:         "Preserve shipment measurements between requests.",
			RequiredBehaviors: []string{"Retain each measurement with its stable identifier."},
		}},
	}
	stateLeaf := assemblyline.ApplicationStateFieldLeafInput{
		Authority: authority, AcceptedFields: []assemblyline.ApplicationServiceStateField{},
	}
	recordLeaf := assemblyline.ApplicationRecordFieldLeafInput{
		Authority: authority, ParentName: "measurements",
		AcceptedRecordFields: []assemblyline.ApplicationServiceStateRecordField{},
	}
	constructors := []func() (assemblyline.PortableJob, error){
		func() (assemblyline.PortableJob, error) {
			return assemblyline.NewApplicationStateFieldCoverageJob(stateLeaf)
		},
		func() (assemblyline.PortableJob, error) {
			return assemblyline.NewApplicationStateFieldNameJob(stateLeaf)
		},
		func() (assemblyline.PortableJob, error) {
			return assemblyline.NewApplicationStateFieldKindJob(
				assemblyline.ApplicationStateFieldKindInput{
					Authority: authority, AcceptedFields: []assemblyline.ApplicationServiceStateField{},
					FocusedName: "measurements",
				},
			)
		},
		func() (assemblyline.PortableJob, error) {
			return assemblyline.NewApplicationRecordFieldCoverageJob(recordLeaf)
		},
		func() (assemblyline.PortableJob, error) {
			return assemblyline.NewApplicationRecordFieldNameJob(recordLeaf)
		},
		func() (assemblyline.PortableJob, error) {
			return assemblyline.NewApplicationRecordFieldKindJob(
				assemblyline.ApplicationRecordFieldKindInput{
					Authority: authority, ParentName: "measurements",
					AcceptedRecordFields: []assemblyline.ApplicationServiceStateRecordField{},
					FocusedName:          "label",
				},
			)
		},
	}
	for _, construct := range constructors {
		job, err := construct()
		if err != nil {
			t.Fatal(err)
		}
		got, err := StationForPortableJob(job)
		if err != nil {
			t.Fatal(err)
		}
		if got != station.CodingServiceStateInterface {
			t.Fatalf("kind=%q station=%q want=%q", job.Kind, got, station.CodingServiceStateInterface)
		}
	}
}

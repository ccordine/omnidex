package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func TestDatabaseCognitionPortableWorkHasOneExactStation(t *testing.T) {
	wants := map[assemblyline.WorkKind]station.ID{
		assemblyline.WorkDatabaseSchemaRelationInventory:         station.DatabaseSchemaSelection,
		assemblyline.WorkDatabaseSchemaRelationNecessity:         station.DatabaseSchemaSelection,
		assemblyline.WorkDatabaseSchemaRelationResolution:        station.DatabaseSchemaSelection,
		assemblyline.WorkDatabaseQueryFromRelation:               station.DatabaseQueryIntent,
		assemblyline.WorkDatabaseQueryShape:                      station.DatabaseQueryIntent,
		assemblyline.WorkDatabaseQueryPurposeInventory:           station.DatabaseQueryIntent,
		assemblyline.WorkDatabaseQueryPurposeNecessity:           station.DatabaseQueryIntent,
		assemblyline.WorkDatabaseQueryPurposeRelation:            station.DatabaseQueryIntent,
		assemblyline.WorkDatabaseQueryProjectionAggregate:        station.DatabaseQueryIntent,
		assemblyline.WorkDatabaseQueryProjectionField:            station.DatabaseQueryIntent,
		assemblyline.WorkDatabaseQueryProjectionTimeBucket:       station.DatabaseQueryIntent,
		assemblyline.WorkDatabaseQueryFilterField:                station.DatabaseQueryIntent,
		assemblyline.WorkDatabaseQueryFilterOperator:             station.DatabaseQueryIntent,
		assemblyline.WorkDatabaseQueryFilterValue:                station.DatabaseQueryIntent,
		assemblyline.WorkDatabaseQueryWindowField:                station.DatabaseQueryIntent,
		assemblyline.WorkDatabaseQueryWindowUnit:                 station.DatabaseQueryIntent,
		assemblyline.WorkDatabaseQueryWindowAmount:               station.DatabaseQueryIntent,
		assemblyline.WorkDatabaseQueryExistenceRelation:          station.DatabaseQueryIntent,
		assemblyline.WorkDatabaseQueryExistenceNegated:           station.DatabaseQueryIntent,
		assemblyline.WorkDatabaseQueryHavingAggregate:            station.DatabaseQueryIntent,
		assemblyline.WorkDatabaseQueryHavingField:                station.DatabaseQueryIntent,
		assemblyline.WorkDatabaseQueryHavingOperator:             station.DatabaseQueryIntent,
		assemblyline.WorkDatabaseQueryHavingValue:                station.DatabaseQueryIntent,
		assemblyline.WorkDatabaseQueryOrderProjection:            station.DatabaseQueryIntent,
		assemblyline.WorkDatabaseQueryOrderDirection:             station.DatabaseQueryIntent,
		assemblyline.WorkDatabaseJoinPathSelection:               station.DatabaseJoinPathSelection,
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

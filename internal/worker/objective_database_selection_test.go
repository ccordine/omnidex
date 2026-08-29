package worker

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/datasource"
)

type maximumDatabaseSchemaSelectionStations struct {
	*scriptedObjectiveDatabaseStations
}

func TestDatabaseSchemaCandidateDescriptorProjectsSemanticRelationsWithoutNestedIDs(t *testing.T) {
	t.Parallel()
	snapshot, err := datasource.NewSchemaSnapshot("source-selection", "selection", []datasource.RelationDefinition{
		{
			Schema: "public", Name: "clinics", Kind: datasource.RelationTable,
			Columns: []datasource.ColumnDefinition{{
				Name: "id", Ordinal: 1, DataType: "uuid", TypeCategory: datasource.TypeUUID,
			}}, PrimaryKey: []string{"id"},
		},
		{
			Schema: "public", Name: "appointments", Kind: datasource.RelationTable,
			Columns: []datasource.ColumnDefinition{
				{Name: "id", Ordinal: 1, DataType: "uuid", TypeCategory: datasource.TypeUUID},
				{Name: "clinic_id", Ordinal: 2, DataType: "uuid", TypeCategory: datasource.TypeUUID},
			}, PrimaryKey: []string{"id"}, ForeignKeys: []datasource.ForeignKeyDefinition{{
				Name: "appointments_clinic", Columns: []string{"clinic_id"},
				ReferencedSchema: "public", ReferencedRelation: "clinics", ReferencedColumns: []string{"id"},
			}},
		},
	}, time.Unix(1_700_000_000, 0))
	if err != nil {
		t.Fatal(err)
	}
	var appointments datasource.SchemaRelation
	for _, relation := range snapshot.Relations {
		if relation.Name == "appointments" {
			appointments = relation
		}
	}
	descriptor, err := objectiveDatabaseRelationDescriptor(snapshot, appointments.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, visible := range []string{
		"RELATION label=public.appointments kind=table",
		"FIELD label=clinic_id type=uuid nullable=false",
		"FOREIGN KEY fields=clinic_id references=public.clinics fields=id",
	} {
		if !strings.Contains(descriptor, visible) {
			t.Fatalf("descriptor omitted semantic relation fact %q:\n%s", visible, descriptor)
		}
	}
	for _, relation := range snapshot.Relations {
		if strings.Contains(descriptor, relation.ID) {
			t.Fatalf("descriptor exposed relation ID %q:\n%s", relation.ID, descriptor)
		}
		for _, column := range relation.Columns {
			if strings.Contains(descriptor, column.ID) {
				t.Fatalf("descriptor exposed field ID %q:\n%s", column.ID, descriptor)
			}
		}
		for _, foreignKey := range relation.ForeignKeys {
			if strings.Contains(descriptor, foreignKey.ID) {
				t.Fatalf("descriptor exposed foreign-key ID %q:\n%s", foreignKey.ID, descriptor)
			}
		}
	}
}

func (station *maximumDatabaseSchemaSelectionStations) SelectSchema(
	_ context.Context,
	input assemblyline.DatabaseSchemaSelectionInput,
) (assemblyline.DatabaseSchemaSelectionDecision, objectiveStationReceipt, error) {
	ids := make([]string, input.MaxSelections)
	for index := range ids {
		ids[index] = input.Candidates[index].RelationID
	}
	return assemblyline.DatabaseSchemaSelectionDecision{
		Schema: assemblyline.DatabaseSchemaSelectionV1, EvidenceNeedID: input.EvidenceNeedID,
		RelationIDs: ids,
	}, objectiveStationReceipt{Calls: exactSemanticLeafCalls}, nil
}

func TestDatabaseSchemaReductionHasOneHardSemanticCallBound(t *testing.T) {
	candidates := make([]assemblyline.DatabaseSchemaCandidate, 2048)
	for index := range candidates {
		candidates[index] = assemblyline.DatabaseSchemaCandidate{
			RelationID: fmt.Sprintf("rel_%04d", index),
			Descriptor: fmt.Sprintf("schema relation %04d", index),
		}
	}
	station := &maximumDatabaseSchemaSelectionStations{
		scriptedObjectiveDatabaseStations: &scriptedObjectiveDatabaseStations{t: t},
	}
	_, calls, err := reduceObjectiveDatabaseCandidates(
		context.Background(), "need-1", "Find the exact relevant records.",
		assemblyline.ObjectiveContext{}, candidates, station,
	)
	if err == nil || !strings.Contains(err.Error(), "96-call semantic reduction bound") {
		t.Fatalf("schema reduction error=%v", err)
	}
	if calls != maxDatabaseSchemaSelectionModelCalls {
		t.Fatalf("schema reduction calls=%d want %d", calls, maxDatabaseSchemaSelectionModelCalls)
	}
}

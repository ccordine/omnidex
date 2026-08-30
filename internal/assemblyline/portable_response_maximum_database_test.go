package assemblyline

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/datasource"
)

func TestPortableResponseMaximumBoundsDatabaseSchemaRelationInventory(t *testing.T) {
	input, err := ProjectDatabaseSchemaRelationInventoryInput(databaseSchemaSelectionFixture())
	if err != nil {
		t.Fatal(err)
	}
	job, err := NewDatabaseSchemaRelationInventoryJob(input)
	if err != nil {
		t.Fatal(err)
	}
	assertPortableResponseMaximum(t, job, maxDatabaseSchemaRelationInventoryBytes)
}

func TestPortableResponseMaximumUsesRegisteredDatabaseSchemaIDs(t *testing.T) {
	authority := databaseSchemaSelectionFixture()
	authority.Candidates[0].RelationID = strings.Repeat("r", 91)
	input := DatabaseSchemaRelationResolutionInput{
		Candidate:  "The clinic appointment records.",
		Candidates: authority.Candidates,
	}
	job, err := NewDatabaseSchemaRelationResolutionJob(input)
	if err != nil {
		t.Fatal(err)
	}
	assertPortableResponseMaximum(t, job, len(authority.Candidates[0].RelationID))
}

func TestPortableResponseMaximumUsesBoundedDatabasePurposeContracts(t *testing.T) {
	authority := DatabaseQueryPurposeAuthority{
		State:      readyDatabaseMaximumState(t, datasource.ResultRecords),
		Collection: DatabaseQueryFilterPurpose,
	}
	inventoryJob, err := NewDatabaseQueryPurposeInventoryJob(authority)
	if err != nil {
		t.Fatal(err)
	}
	assertPortableResponseMaximum(t, inventoryJob, maxDatabaseQueryPurposeInventoryBytes)

	inventory, err := DecodeDatabaseQueryPurposeInventory(authority, "match the requested state")
	if err != nil {
		t.Fatal(err)
	}
	necessityJob, err := NewDatabaseQueryPurposeNecessityJob(DatabaseQueryPurposeNecessityInput{
		Authority: authority, Inventory: inventory, CandidateIndex: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertPortableResponseMaximum(t, necessityJob, len(DatabaseQueryPurposeNotNecessary))
}

func TestPortableResponseMaximumFiltersPayloadAllowedValuesByType(t *testing.T) {
	state, fieldID := enumDatabaseMaximumState(
		t, datasource.TypeBoolean, []string{"false", "not-a-boolean-value"},
	)
	input := DatabaseQueryFilterLeafInput{
		State: state, AcceptedFilters: []datasource.RelationalPredicate{},
		Purpose: "match the requested state",
		FieldID: fieldID, Operator: datasource.FilterEqual,
		AcceptedValues: []datasource.IntentLiteral{},
	}
	job, err := NewDatabaseQueryFilterValueJob(input)
	if err != nil {
		t.Fatal(err)
	}
	assertPortableResponseMaximum(t, job, len("false"))
}

func TestPortableResponseMaximumUsesDatabaseLiteralTypeBounds(t *testing.T) {
	for _, fixture := range []struct {
		name     string
		category datasource.ColumnTypeCategory
		want     int
	}{
		{name: "text", category: datasource.TypeText, want: datasource.MaxIntentStringLiteralBytes},
		{name: "integer", category: datasource.TypeInteger, want: 20},
		{name: "decimal", category: datasource.TypeDecimal, want: datasource.MaxIntentDecimalLiteralBytes},
		{name: "boolean", category: datasource.TypeBoolean, want: len("false")},
		{name: "date", category: datasource.TypeDate, want: len("2006-01-02")},
		{name: "UUID", category: datasource.TypeUUID, want: 36},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			state, fieldID := enumDatabaseMaximumState(t, fixture.category, nil)
			input := DatabaseQueryFilterLeafInput{
				State: state, AcceptedFilters: []datasource.RelationalPredicate{},
				Purpose: "match the requested value",
				FieldID: fieldID, Operator: datasource.FilterEqual,
				AcceptedValues: []datasource.IntentLiteral{},
			}
			job, err := NewDatabaseQueryFilterValueJob(input)
			if err != nil {
				t.Fatal(err)
			}
			assertPortableResponseMaximum(t, job, fixture.want)
		})
	}
}

func TestPortableResponseMaximumIncludesDescendingDatabaseOrder(t *testing.T) {
	state := readyDatabaseMaximumState(t, datasource.ResultRecords)
	state.Projections = []datasource.RelationalProjection{{
		FieldID: state.Authority.SchemaProjection.Relations[0].Columns[0].ID,
	}}
	projection := 0
	job, err := NewDatabaseQueryOrderDirectionJob(DatabaseQueryOrderLeafInput{
		State: state, Purpose: "sort by the requested projection", Projection: &projection,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertPortableResponseMaximum(t, job, len(datasource.OrderDescending))
}

func readyDatabaseMaximumState(
	t *testing.T,
	shape datasource.ResultShape,
) DatabaseQueryIntentLeafState {
	t.Helper()
	authority, _, _ := databaseQueryIntentFixture(t)
	state := NewDatabaseQueryIntentLeafState(authority)
	state.FromRelationID = authority.SchemaProjection.Relations[0].ID
	state.Shape = shape
	return state
}

func enumDatabaseMaximumState(
	t *testing.T,
	category datasource.ColumnTypeCategory,
	allowed []string,
) (DatabaseQueryIntentLeafState, string) {
	t.Helper()
	state := readyDatabaseMaximumState(t, datasource.ResultRecords)
	column := &state.Authority.SchemaProjection.Relations[0].Columns[0]
	column.TypeCategory = category
	column.AllowedValues = append([]string(nil), allowed...)
	return state, column.ID
}

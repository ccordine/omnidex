package assemblyline

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/datasource"
)

func TestPortableResponseMaximumUsesRemainingDatabaseSchemaIDs(t *testing.T) {
	authority := databaseSchemaSelectionFixture()
	authority.Candidates[0].RelationID = strings.Repeat("r", 91)
	input := DatabaseSchemaSelectionLeafInput{
		Authority: authority, SelectedRelationIDs: []string{"rel_b"},
	}
	job, err := NewDatabaseSchemaRelationSelectionJob(input)
	if err != nil {
		t.Fatal(err)
	}
	assertPortableResponseMaximum(t, job, len(authority.Candidates[0].RelationID))
}

func TestPortableResponseMaximumUsesExactDatabaseCoverageState(t *testing.T) {
	incomplete := readyDatabaseMaximumState(t, datasource.ResultRanking)
	incompleteJob, err := NewDatabaseQueryProjectionCoverageJob(incomplete)
	if err != nil {
		t.Fatal(err)
	}
	assertPortableResponseMaximum(t, incompleteJob, len(DatabaseQueryItemRemains))

	complete := readyDatabaseMaximumState(t, datasource.ResultRecords)
	complete.Projections = []datasource.RelationalProjection{{
		FieldID: complete.Authority.SchemaProjection.Relations[0].Columns[0].ID,
	}}
	completeJob, err := NewDatabaseQueryProjectionCoverageJob(complete)
	if err != nil {
		t.Fatal(err)
	}
	assertPortableResponseMaximum(t, completeJob, len(DatabaseQueryNoUncoveredItem))
}

func TestPortableResponseMaximumUsesExactDatabaseFilterValueCoverageState(t *testing.T) {
	state, fieldID := enumDatabaseMaximumState(t, datasource.TypeText, []string{"open", "closed"})
	empty := DatabaseQueryFilterLeafInput{
		State: state, AcceptedFilters: []datasource.RelationalPredicate{},
		FieldID: fieldID, Operator: datasource.FilterIn,
		AcceptedValues: []datasource.IntentLiteral{},
	}
	emptyJob, err := NewDatabaseQueryFilterValueCoverageJob(empty)
	if err != nil {
		t.Fatal(err)
	}
	assertPortableResponseMaximum(t, emptyJob, len(DatabaseQueryValueRemains))

	retained := empty
	retained.AcceptedValues = []datasource.IntentLiteral{{
		Type: datasource.LiteralString, Value: "open",
	}}
	retainedJob, err := NewDatabaseQueryFilterValueCoverageJob(retained)
	if err != nil {
		t.Fatal(err)
	}
	assertPortableResponseMaximum(t, retainedJob, len(DatabaseQueryNoUncoveredValue))
}

func TestPortableResponseMaximumFiltersPayloadAllowedValuesByType(t *testing.T) {
	state, fieldID := enumDatabaseMaximumState(
		t, datasource.TypeBoolean, []string{"false", "not-a-boolean-value"},
	)
	input := DatabaseQueryFilterLeafInput{
		State: state, AcceptedFilters: []datasource.RelationalPredicate{},
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
		State: state, Projection: &projection,
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

package assemblyline

import (
	"testing"

	"github.com/gryph/omnidex/internal/datasource"
)

func databaseSelectionStateFixture(relation, value, timestamp string) DatabaseQueryIntentLeafState {
	state := withUnrelatedAcceptedQueryClauses(databaseParameterStateFixture(relation, value, timestamp))
	state.Authority.SchemaProjection.Relations = append(state.Authority.SchemaProjection.Relations,
		datasource.IntentRelationProjection{
			ID: "remaining", SchemaName: "public", Name: "remaining_records", Kind: datasource.RelationTable,
			Columns: []datasource.IntentColumnProjection{{ID: "remaining-id", Name: "id", TypeCategory: datasource.TypeInteger}},
		})
	state.Projections = append(state.Projections, datasource.RelationalProjection{FieldID: "focus-time"})
	return state
}

func changeUnrelatedSelectionClauses(state *DatabaseQueryIntentLeafState) {
	state.Projections[1].Aggregate = datasource.AggregateSum
	state.Filters[0].Values[0].Value = "changed-retained-private-value"
	state.TemporalWindows[0].Amount = 997
	state.Exists[0].Negated = true
	state.Exists[0].Filters = []datasource.RelationalPredicate{{
		FieldID: "other-id", Operator: datasource.FilterLT,
		Values: []datasource.IntentLiteral{{Type: datasource.LiteralInteger, Value: "731"}},
	}}
	state.Having[0].Value.Value = "972"
	state.OrderBy[0].Direction = datasource.OrderAscending
}

func databaseSelectionCases(t *testing.T, state DatabaseQueryIntentLeafState) []databaseParameterCase {
	t.Helper()
	value := state.Authority.SchemaProjection.Relations[0].Columns[0].Name
	timestamp := state.Authority.SchemaProjection.Relations[0].Columns[1].Name
	projection := DatabaseQueryProjectionLeafInput{State: state, Purpose: "Show the total " + value + "."}
	projectionField := projection
	projectionField.Aggregate = datasource.AggregateSum
	filter := DatabaseQueryFilterLeafInput{
		State: state, Purpose: "Require " + value + " to exceed seven.", ParentPurpose: "Include qualifying records.",
		AcceptedFilters: append([]datasource.RelationalPredicate{}, state.Filters...), AcceptedValues: []datasource.IntentLiteral{},
	}
	window := DatabaseQueryWindowLeafInput{State: state, Purpose: "Include the previous day measured on " + timestamp + "."}
	existence := DatabaseQueryExistenceLeafInput{
		State: state, Purpose: "Matching remaining records must exist.", Filters: []datasource.RelationalPredicate{},
	}
	having := DatabaseQueryHavingLeafInput{State: state, Purpose: "Require the sum of " + value + " to exceed seven."}
	havingField := having
	havingField.Aggregate = datasource.AggregateSum
	order := DatabaseQueryOrderLeafInput{State: state, Purpose: "Order by " + value + "."}
	return []databaseParameterCase{
		databaseParameterCaseFor(t, projection, NewDatabaseQueryProjectionAggregateJob, DecodeDatabaseQueryProjectionAggregateLeaf,
			"E", datasource.AggregateSum, projection.Purpose, value),
		databaseParameterCaseFor(t, projectionField, NewDatabaseQueryProjectionFieldJob, DecodeDatabaseQueryProjectionFieldLeaf,
			"A", "focus-value", projection.Purpose, value, "sum numeric field values"),
		databaseParameterCaseFor(t, filter, NewDatabaseQueryFilterFieldJob, DecodeDatabaseQueryFilterFieldLeaf,
			"A", "focus-value", filter.Purpose, filter.ParentPurpose, value),
		databaseParameterCaseFor(t, window, NewDatabaseQueryWindowFieldJob, DecodeDatabaseQueryWindowFieldLeaf,
			"A", "focus-time", window.Purpose, timestamp),
		databaseParameterCaseFor(t, existence, NewDatabaseQueryExistenceRelationJob, DecodeDatabaseQueryExistenceRelationLeaf,
			"B", "remaining", existence.Purpose, "public.remaining_records"),
		databaseParameterCaseFor(t, having, NewDatabaseQueryHavingAggregateJob, DecodeDatabaseQueryHavingAggregateLeaf,
			"D", datasource.AggregateSum, having.Purpose, value),
		databaseParameterCaseFor(t, havingField, NewDatabaseQueryHavingFieldJob, DecodeDatabaseQueryHavingFieldLeaf,
			"A", "focus-value", having.Purpose, value, "sum numeric field values"),
		databaseParameterCaseFor(t, order, NewDatabaseQueryOrderProjectionJob, DecodeDatabaseQueryOrderProjectionLeaf,
			"A", 0, order.Purpose, value),
	}
}

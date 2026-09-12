package assemblyline

import (
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/datasource"
)

func databaseParameterStateFixture(relation, value, timestamp string) DatabaseQueryIntentLeafState {
	state := NewDatabaseQueryIntentLeafState(DatabaseQueryIntentInput{
		EvidenceNeedID: "need-private", ExactNeed: "The complete request stays in code.",
		Context: ObjectiveContext{}, TemporalAsOf: "2026-09-08T12:00:00Z", MaxRows: 50,
		SchemaProjection: datasource.IntentSchemaProjection{
			Schema: datasource.IntentSchemaProjectionV1, SourceID: "source-private",
			Relations: []datasource.IntentRelationProjection{
				{ID: "focus", SchemaName: "public", Name: relation, Kind: datasource.RelationTable,
					Columns: []datasource.IntentColumnProjection{
						{ID: "focus-value", Name: value, TypeCategory: datasource.TypeInteger},
						{ID: "focus-time", Name: timestamp, TypeCategory: datasource.TypeTemporal},
						{ID: "prior-value", Name: "unrelated_measure", TypeCategory: datasource.TypeInteger},
						{ID: "prior-time", Name: "unrelated_timestamp", TypeCategory: datasource.TypeTemporal},
						{ID: "prior-text", Name: "unrelated_note", TypeCategory: datasource.TypeText},
					}},
				{ID: "other", SchemaName: "public", Name: "unrelated_records", Kind: datasource.RelationTable,
					Columns: []datasource.IntentColumnProjection{
						{ID: "other-id", Name: "id", TypeCategory: datasource.TypeInteger},
					}},
			},
		},
	})
	state.FromRelationID = "focus"
	state.Shape = datasource.ResultRecords
	state.Projections = []datasource.RelationalProjection{{FieldID: "focus-value"}}
	return state
}

func withUnrelatedAcceptedQueryClauses(state DatabaseQueryIntentLeafState) DatabaseQueryIntentLeafState {
	state.Projections = append(append([]datasource.RelationalProjection{}, state.Projections...),
		datasource.RelationalProjection{FieldID: "prior-value"})
	state.Filters = []datasource.RelationalPredicate{{
		FieldID: "prior-text", Operator: datasource.FilterEqual,
		Values: []datasource.IntentLiteral{{Type: datasource.LiteralString, Value: "unrelated-value"}},
	}}
	state.TemporalWindows = []DatabaseTemporalWindowDecision{{FieldID: "prior-time", Unit: datasource.WindowYear, Amount: 73}}
	state.Exists = []datasource.ExistencePredicate{{RelationID: "other", Filters: []datasource.RelationalPredicate{}}}
	state.Having = []datasource.AggregatePredicate{{
		Aggregate: datasource.AggregateSum, FieldID: "prior-value", Operator: datasource.FilterGT,
		Value: datasource.IntentLiteral{Type: datasource.LiteralInteger, Value: "391"},
	}}
	state.OrderBy = []datasource.OrderTerm{{Projection: 1, Direction: datasource.OrderDescending}}
	return state
}

type databaseParameterCase struct {
	job      PortableJob
	decode   func() error
	required []string
}

func databaseParameterCaseFor[I any, O comparable](
	t *testing.T, input I, create func(I) (PortableJob, error),
	decode func(I, string) (O, error), raw string, expected O, required ...string,
) databaseParameterCase {
	t.Helper()
	job, err := create(input)
	if err != nil {
		t.Fatal(err)
	}
	return databaseParameterCase{job: job, required: required, decode: func() error {
		actual, err := decode(input, raw)
		if err != nil {
			return err
		}
		if actual != expected {
			return fmt.Errorf("decoded %q as %v; want %v", raw, actual, expected)
		}
		return nil
	}}
}

func databaseParameterCases(t *testing.T, state DatabaseQueryIntentLeafState) []databaseParameterCase {
	t.Helper()
	value := state.Authority.SchemaProjection.Relations[0].Columns[0].Name
	timestamp := state.Authority.SchemaProjection.Relations[0].Columns[1].Name
	projection := DatabaseQueryProjectionLeafInput{State: state, Purpose: "Group " + timestamp + " by month.", FieldID: "focus-time"}
	window := DatabaseQueryWindowLeafInput{State: state, Purpose: "Include the previous five days measured on " + timestamp + ".", FieldID: "focus-time", Unit: datasource.WindowDay}
	filter := DatabaseQueryFilterLeafInput{
		State: state, Purpose: "Require " + value + " to exceed seven.", FieldID: "focus-value",
		AcceptedFilters: append([]datasource.RelationalPredicate{}, state.Filters...), AcceptedValues: []datasource.IntentLiteral{},
	}
	filterValue := filter
	filterValue.Operator = datasource.FilterGT
	having := DatabaseQueryHavingLeafInput{State: state, Purpose: "Require the sum of " + value + " to exceed seven.", Aggregate: datasource.AggregateSum, FieldID: "focus-value"}
	havingValue := having
	havingValue.Operator = datasource.FilterGT
	index := 0
	order := DatabaseQueryOrderLeafInput{State: state, Purpose: "Order " + value + " from largest to smallest.", Projection: &index}
	existence := DatabaseQueryExistenceLeafInput{
		State: state, Purpose: "Matching rows must not exist.", RelationID: "focus", Filters: []datasource.RelationalPredicate{},
	}
	return []databaseParameterCase{
		databaseParameterCaseFor(t, projection, NewDatabaseQueryProjectionTimeBucketJob, DecodeDatabaseQueryProjectionTimeBucketLeaf,
			"D", datasource.BucketMonth, projection.Purpose, timestamp),
		databaseParameterCaseFor(t, window, NewDatabaseQueryWindowUnitJob, DecodeDatabaseQueryWindowUnitLeaf,
			"B", datasource.WindowDay, window.Purpose, timestamp),
		databaseParameterCaseFor(t, window, NewDatabaseQueryWindowAmountJob, DecodeDatabaseQueryWindowAmountLeaf,
			"5", 5, window.Purpose, timestamp, "ACCEPTED WINDOW UNIT"),
		databaseParameterCaseFor(t, filter, NewDatabaseQueryFilterOperatorJob, DecodeDatabaseQueryFilterOperatorLeaf,
			"G", datasource.FilterGT, filter.Purpose, value),
		databaseParameterCaseFor(t, filterValue, NewDatabaseQueryFilterValueJob, DecodeDatabaseQueryFilterValueLeaf,
			"7", datasource.IntentLiteral{Type: datasource.LiteralInteger, Value: "7"}, filterValue.Purpose, value, "ACCEPTED FILTER RELATION"),
		databaseParameterCaseFor(t, having, NewDatabaseQueryHavingOperatorJob, DecodeDatabaseQueryHavingOperatorLeaf,
			"C", datasource.FilterGT, having.Purpose, value),
		databaseParameterCaseFor(t, havingValue, NewDatabaseQueryHavingValueJob, DecodeDatabaseQueryHavingValueLeaf,
			"7", datasource.IntentLiteral{Type: datasource.LiteralInteger, Value: "7"}, havingValue.Purpose, value, "ACCEPTED HAVING RELATION"),
		databaseParameterCaseFor(t, order, NewDatabaseQueryOrderDirectionJob, DecodeDatabaseQueryOrderDirectionLeaf,
			"B", datasource.OrderDescending, order.Purpose, value),
		databaseParameterCaseFor(t, existence, NewDatabaseQueryExistenceNegatedJob, DecodeDatabaseQueryExistenceNegatedLeaf,
			"B", true, existence.Purpose, state.Authority.SchemaProjection.Relations[0].Name),
	}
}

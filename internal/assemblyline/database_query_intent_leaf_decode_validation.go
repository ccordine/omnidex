package assemblyline

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/datasource"
)

func decodeDatabaseQueryRawLeaf(label, raw string) (string, error) {
	return decodeRawSemanticLeaf(label, raw, maxDatabaseQueryLeafBytes, false)
}

// DatabaseQueryProjectionsHaveRequiredShape reports whether the retained
// projections satisfy the minimum code-owned cardinality and shape invariant.
// It does not decide whether an open-ended shape has another semantic item.
func DatabaseQueryProjectionsHaveRequiredShape(state DatabaseQueryIntentLeafState) bool {
	aggregates, plain, buckets := 0, 0, 0
	for _, projection := range state.Projections {
		if projection.Aggregate != "" {
			aggregates++
		} else {
			plain++
		}
		if projection.TimeBucket != "" {
			buckets++
		}
	}
	switch state.Shape {
	case datasource.ResultRecords:
		return len(state.Projections) > 0 && aggregates == 0 && buckets == 0
	case datasource.ResultScalar:
		return len(state.Projections) == 1 && aggregates == 1
	case datasource.ResultRanking, datasource.ResultDistribution, datasource.ResultComparison:
		return aggregates > 0 && plain > 0
	case datasource.ResultTrend:
		return aggregates > 0 && plain > 0 && buckets > 0
	case datasource.ResultExistence:
		return len(state.Projections) == 0
	default:
		return false
	}
}

func validDatabaseQueryProjectionAggregate(operation datasource.AggregateOperation) bool {
	switch operation {
	case datasource.AggregateCountRows, datasource.AggregateCount,
		datasource.AggregateCountDistinct, datasource.AggregateSum,
		datasource.AggregateAverage, datasource.AggregateMinimum,
		datasource.AggregateMaximum:
		return true
	default:
		return false
	}
}

func validateDatabaseQueryProjection(
	state DatabaseQueryIntentLeafState,
	projection datasource.RelationalProjection,
) error {
	intent := databaseQueryValidationIntent(state)
	switch {
	case projection.Aggregate != "":
		intent.Shape = datasource.ResultScalar
		intent.Projections = []datasource.RelationalProjection{projection}
	case projection.TimeBucket != "":
		intent.Shape = datasource.ResultTrend
		intent.Projections = []datasource.RelationalProjection{
			projection, {Aggregate: datasource.AggregateCountRows},
		}
		intent.GroupBy = []int{0}
	default:
		intent.Shape = datasource.ResultRecords
		intent.Projections = []datasource.RelationalProjection{projection}
	}
	return intent.Validate(databaseIntentValidationSnapshot(state.Authority.SchemaProjection))
}

func validateDatabaseQueryPredicate(
	state DatabaseQueryIntentLeafState,
	predicate datasource.RelationalPredicate,
	scopeRelationID string,
) error {
	intent := databaseQueryValidationIntent(state)
	fieldID, err := databaseQueryValidationProjectionField(state)
	if err != nil {
		return err
	}
	intent.Shape = datasource.ResultRecords
	intent.Projections = []datasource.RelationalProjection{{FieldID: fieldID}}
	if scopeRelationID == "" {
		intent.Filters = []datasource.RelationalPredicate{predicate}
	} else {
		intent.Exists = []datasource.ExistencePredicate{{
			RelationID: scopeRelationID, Filters: []datasource.RelationalPredicate{predicate},
		}}
	}
	return intent.Validate(databaseIntentValidationSnapshot(state.Authority.SchemaProjection))
}

func validateDatabaseQueryWindow(
	state DatabaseQueryIntentLeafState,
	window DatabaseTemporalWindowDecision,
) error {
	intent := databaseQueryValidationIntent(state)
	intent.Shape = datasource.ResultRecords
	intent.Projections = []datasource.RelationalProjection{{FieldID: window.FieldID}}
	intent.TemporalWindows = []datasource.TemporalWindow{{
		FieldID: window.FieldID, Unit: window.Unit, Amount: window.Amount,
		AsOf: state.Authority.TemporalAsOf,
	}}
	return intent.Validate(databaseIntentValidationSnapshot(state.Authority.SchemaProjection))
}

func validateDatabaseQueryHaving(
	state DatabaseQueryIntentLeafState,
	predicate datasource.AggregatePredicate,
) error {
	intent := databaseQueryValidationIntent(state)
	intent.Shape = datasource.ResultScalar
	intent.Projections = []datasource.RelationalProjection{{
		Aggregate: predicate.Aggregate, FieldID: predicate.FieldID,
	}}
	intent.Having = []datasource.AggregatePredicate{predicate}
	return intent.Validate(databaseIntentValidationSnapshot(state.Authority.SchemaProjection))
}

func databaseQueryValidationIntent(state DatabaseQueryIntentLeafState) datasource.RelationalIntent {
	return datasource.RelationalIntent{
		Schema:            datasource.RelationalIntentV1,
		SourceID:          state.Authority.SchemaProjection.SourceID,
		SchemaFingerprint: state.Authority.SchemaProjection.SchemaFingerprint,
		FromRelationID:    state.FromRelationID, Limit: 1,
		Projections: []datasource.RelationalProjection{}, Filters: []datasource.RelationalPredicate{},
		TemporalWindows: []datasource.TemporalWindow{}, Exists: []datasource.ExistencePredicate{},
		GroupBy: []int{}, Having: []datasource.AggregatePredicate{}, OrderBy: []datasource.OrderTerm{},
	}
}

func databaseQueryValidationProjectionField(state DatabaseQueryIntentLeafState) (string, error) {
	for _, relation := range state.Authority.SchemaProjection.Relations {
		if relation.ID == state.FromRelationID && len(relation.Columns) > 0 {
			return relation.Columns[0].ID, nil
		}
	}
	return "", fmt.Errorf("database query from relation has no projected validation field")
}

func databaseQueryLiteral(
	state DatabaseQueryIntentLeafState,
	fieldID string,
	value string,
) (datasource.IntentLiteral, error) {
	column, _, ok := databaseQueryColumn(state, fieldID)
	if !ok {
		return datasource.IntentLiteral{}, fmt.Errorf("database query literal field %q was not projected", fieldID)
	}
	literalType := map[datasource.ColumnTypeCategory]datasource.LiteralType{
		datasource.TypeInteger:  datasource.LiteralInteger,
		datasource.TypeText:     datasource.LiteralString,
		datasource.TypeBoolean:  datasource.LiteralBoolean,
		datasource.TypeTemporal: datasource.LiteralTimestamp,
		datasource.TypeDate:     datasource.LiteralDate,
		datasource.TypeUUID:     datasource.LiteralUUID,
	}[column.TypeCategory]
	if column.TypeCategory == datasource.TypeDecimal {
		literalType = datasource.LiteralInteger
		if strings.Contains(value, ".") {
			literalType = datasource.LiteralDecimal
		}
	}
	if literalType == "" {
		return datasource.IntentLiteral{}, fmt.Errorf("database query field type %q cannot bind a literal", column.TypeCategory)
	}
	return datasource.IntentLiteral{Type: literalType, Value: value}, nil
}

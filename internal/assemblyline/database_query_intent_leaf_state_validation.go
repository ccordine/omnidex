package assemblyline

import (
	"fmt"

	"github.com/gryph/omnidex/internal/datasource"
)

func validateDatabaseQueryPartialState(state DatabaseQueryIntentLeafState) error {
	for index, projection := range state.Projections {
		if err := validateDatabaseQueryProjection(state, projection); err != nil {
			return fmt.Errorf("database query accepted projection %d: %w", index, err)
		}
	}
	for index, predicate := range state.Filters {
		if err := validateDatabaseQueryPredicate(state, predicate, ""); err != nil {
			return fmt.Errorf("database query accepted filter %d: %w", index, err)
		}
	}
	for index, window := range state.TemporalWindows {
		if err := validateDatabaseQueryWindow(state, window); err != nil {
			return fmt.Errorf("database query accepted window %d: %w", index, err)
		}
	}
	for index, exists := range state.Exists {
		intent := databaseQueryValidationIntent(state)
		fieldID, err := databaseQueryValidationProjectionField(state)
		if err != nil {
			return err
		}
		intent.Shape = datasource.ResultRecords
		intent.Projections = []datasource.RelationalProjection{{FieldID: fieldID}}
		intent.Exists = []datasource.ExistencePredicate{exists}
		if err := intent.Validate(databaseIntentValidationSnapshot(state.Authority.SchemaProjection)); err != nil {
			return fmt.Errorf("database query accepted existence predicate %d: %w", index, err)
		}
	}
	for index, predicate := range state.Having {
		if err := validateDatabaseQueryHaving(state, predicate); err != nil {
			return fmt.Errorf("database query accepted having predicate %d: %w", index, err)
		}
	}
	seenOrder := map[int]struct{}{}
	for index, term := range state.OrderBy {
		if term.Projection < 0 || term.Projection >= len(state.Projections) ||
			term.Direction != datasource.OrderAscending && term.Direction != datasource.OrderDescending {
			return fmt.Errorf("database query accepted order term %d is invalid", index)
		}
		if _, duplicate := seenOrder[term.Projection]; duplicate {
			return fmt.Errorf("database query accepted order term repeats projection %d", term.Projection)
		}
		seenOrder[term.Projection] = struct{}{}
	}
	return nil
}

func (state DatabaseQueryIntentLeafState) validateReady() error {
	if err := state.validate(); err != nil {
		return err
	}
	if state.FromRelationID == "" || state.Shape == "" {
		return fmt.Errorf("database query leaf requires accepted from-relation and shape authority")
	}
	return nil
}

func databaseQueryRelationExists(state DatabaseQueryIntentLeafState, id string) bool {
	for _, relation := range state.Authority.SchemaProjection.Relations {
		if relation.ID == id {
			return true
		}
	}
	return false
}

func databaseQueryColumn(
	state DatabaseQueryIntentLeafState,
	id string,
) (datasource.IntentColumnProjection, string, bool) {
	for _, relation := range state.Authority.SchemaProjection.Relations {
		for _, column := range relation.Columns {
			if column.ID == id {
				return column, relation.ID, true
			}
		}
	}
	return datasource.IntentColumnProjection{}, "", false
}

func validDatabaseQueryShape(shape datasource.ResultShape) bool {
	switch shape {
	case datasource.ResultRecords, datasource.ResultScalar, datasource.ResultRanking,
		datasource.ResultDistribution, datasource.ResultComparison,
		datasource.ResultTrend, datasource.ResultExistence:
		return true
	default:
		return false
	}
}

func validDatabaseQueryFilterOperator(operator datasource.FilterOperator) bool {
	switch operator {
	case datasource.FilterEqual, datasource.FilterNotEqual, datasource.FilterGT,
		datasource.FilterGTE, datasource.FilterLT, datasource.FilterLTE,
		datasource.FilterIn, datasource.FilterNotIn, datasource.FilterIsNull,
		datasource.FilterIsNotNull, datasource.FilterContains, datasource.FilterPrefix:
		return true
	default:
		return false
	}
}

func validDatabaseQueryWindowUnit(unit datasource.WindowUnit) bool {
	switch unit {
	case datasource.WindowHour, datasource.WindowDay, datasource.WindowWeek,
		datasource.WindowMonth, datasource.WindowYear:
		return true
	default:
		return false
	}
}

func validDatabaseQueryHavingAggregate(operation datasource.AggregateOperation) bool {
	switch operation {
	case datasource.AggregateCountRows, datasource.AggregateCount,
		datasource.AggregateCountDistinct, datasource.AggregateSum,
		datasource.AggregateAverage:
		return true
	default:
		return false
	}
}

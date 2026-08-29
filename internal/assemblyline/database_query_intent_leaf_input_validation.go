package assemblyline

import (
	"fmt"

	"github.com/gryph/omnidex/internal/datasource"
)

func (input DatabaseQueryProjectionLeafInput) validate() error {
	return input.State.validateReady()
}

func (input DatabaseQueryProjectionLeafInput) validateForField() error {
	if err := input.validate(); err != nil {
		return err
	}
	if input.Aggregate == datasource.AggregateCountRows ||
		input.Aggregate != "" && !validDatabaseQueryProjectionAggregate(input.Aggregate) {
		return fmt.Errorf("database query projection aggregate %q does not accept a field", input.Aggregate)
	}
	return nil
}

func (input DatabaseQueryProjectionLeafInput) validateForTimeBucket() error {
	if err := input.validateForField(); err != nil {
		return err
	}
	if input.Aggregate != "" {
		return fmt.Errorf("database query aggregate projection cannot select a time bucket")
	}
	column, _, ok := databaseQueryColumn(input.State, input.FieldID)
	if !ok || column.TypeCategory != datasource.TypeTemporal && column.TypeCategory != datasource.TypeDate {
		return fmt.Errorf("database query time bucket field %q is not temporal", input.FieldID)
	}
	return nil
}

func (input DatabaseQueryFilterLeafInput) validate() error {
	if err := input.State.validateReady(); err != nil {
		return err
	}
	if input.AcceptedFilters == nil || input.AcceptedValues == nil {
		return fmt.Errorf("database query filter partial collections must be explicit")
	}
	if len(input.AcceptedFilters) > datasource.MaxIntentFilters ||
		len(input.AcceptedValues) > datasource.MaxIntentFilterValues {
		return fmt.Errorf("database query filter partial state exceeds a collection bound")
	}
	if input.ScopeRelationID != "" && !databaseQueryRelationExists(input.State, input.ScopeRelationID) {
		return fmt.Errorf("database query filter scope relation %q was not projected", input.ScopeRelationID)
	}
	for index, predicate := range input.AcceptedFilters {
		if err := validateDatabaseQueryPredicate(input.State, predicate, input.ScopeRelationID); err != nil {
			return fmt.Errorf("database query accepted scoped filter %d: %w", index, err)
		}
	}
	if len(input.AcceptedValues) > 0 {
		if _, relationID, ok := databaseQueryColumn(input.State, input.FieldID); !ok ||
			input.ScopeRelationID != "" && relationID != input.ScopeRelationID {
			return fmt.Errorf("database query filter values lack one valid field authority")
		}
		if input.Operator != datasource.FilterIn && input.Operator != datasource.FilterNotIn {
			return fmt.Errorf("database query accepted values require one set-membership operator")
		}
		if err := validateDatabaseQueryPredicate(input.State, datasource.RelationalPredicate{
			FieldID: input.FieldID, Operator: input.Operator, Values: input.AcceptedValues,
		}, input.ScopeRelationID); err != nil {
			return err
		}
	}
	return nil
}

func (input DatabaseQueryFilterLeafInput) validateField() error {
	if err := input.validate(); err != nil {
		return err
	}
	_, relationID, ok := databaseQueryColumn(input.State, input.FieldID)
	if !ok || input.ScopeRelationID != "" && relationID != input.ScopeRelationID {
		return fmt.Errorf("database query filter field ID %q is outside its authority", input.FieldID)
	}
	return nil
}

func (input DatabaseQueryFilterLeafInput) validateOperator() error {
	if err := input.validateField(); err != nil {
		return err
	}
	if !validDatabaseQueryFilterOperator(input.Operator) {
		return fmt.Errorf("database query filter operator %q is not registered", input.Operator)
	}
	return nil
}

func (input DatabaseQueryWindowLeafInput) validate() error { return input.State.validateReady() }

func (input DatabaseQueryWindowLeafInput) validateField() error {
	if err := input.validate(); err != nil {
		return err
	}
	column, _, ok := databaseQueryColumn(input.State, input.FieldID)
	if !ok || column.TypeCategory != datasource.TypeTemporal && column.TypeCategory != datasource.TypeDate {
		return fmt.Errorf("database query window field %q is not temporal", input.FieldID)
	}
	return nil
}

func (input DatabaseQueryWindowLeafInput) validateUnit() error {
	if err := input.validateField(); err != nil {
		return err
	}
	if !validDatabaseQueryWindowUnit(input.Unit) {
		return fmt.Errorf("database query window unit %q is not registered", input.Unit)
	}
	return nil
}

func (input DatabaseQueryExistenceLeafInput) validate() error {
	if err := input.State.validateReady(); err != nil {
		return err
	}
	if input.Filters == nil || len(input.Filters) > datasource.MaxIntentFilters {
		return fmt.Errorf("database query existence filters must be explicit and bounded")
	}
	if len(input.Filters) > 0 {
		if !databaseQueryRelationExists(input.State, input.RelationID) {
			return fmt.Errorf("database query existence filters lack one relation authority")
		}
		for index, predicate := range input.Filters {
			if err := validateDatabaseQueryPredicate(input.State, predicate, input.RelationID); err != nil {
				return fmt.Errorf("database query existence filter %d: %w", index, err)
			}
		}
	}
	return nil
}

func (input DatabaseQueryExistenceLeafInput) validateRelation() error {
	if err := input.validate(); err != nil {
		return err
	}
	if !databaseQueryRelationExists(input.State, input.RelationID) {
		return fmt.Errorf("database query existence relation %q was not projected", input.RelationID)
	}
	return nil
}

func (input DatabaseQueryHavingLeafInput) validate() error { return input.State.validateReady() }

func (input DatabaseQueryHavingLeafInput) validateAggregate() error {
	if err := input.validate(); err != nil {
		return err
	}
	if !validDatabaseQueryHavingAggregate(input.Aggregate) {
		return fmt.Errorf("database query having aggregate %q is not registered", input.Aggregate)
	}
	return nil
}

func (input DatabaseQueryHavingLeafInput) validateField() error {
	if err := input.validateAggregate(); err != nil {
		return err
	}
	if input.Aggregate != datasource.AggregateCountRows {
		if _, _, ok := databaseQueryColumn(input.State, input.FieldID); !ok {
			return fmt.Errorf("database query having field %q was not projected", input.FieldID)
		}
	}
	return nil
}

func (input DatabaseQueryHavingLeafInput) validateOperator() error {
	if err := input.validateField(); err != nil {
		return err
	}
	switch input.Operator {
	case datasource.FilterEqual, datasource.FilterNotEqual, datasource.FilterGT,
		datasource.FilterGTE, datasource.FilterLT, datasource.FilterLTE:
		return nil
	default:
		return fmt.Errorf("database query having operator %q is not registered", input.Operator)
	}
}

func (input DatabaseQueryOrderLeafInput) validate() error { return input.State.validateReady() }

func (input DatabaseQueryOrderLeafInput) validateProjection() error {
	if err := input.validate(); err != nil {
		return err
	}
	if input.Projection == nil || *input.Projection < 0 || *input.Projection >= len(input.State.Projections) {
		return fmt.Errorf("database query order projection is outside accepted projections")
	}
	return nil
}

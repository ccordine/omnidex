package datasource

import (
	"fmt"
	"time"
)

func (intent RelationalIntent) Validate(snapshot SchemaSnapshot) error {
	if snapshot.Schema != SchemaSnapshotV1 || snapshot.Driver != DriverPostgres || snapshot.Fingerprint == "" {
		return fmt.Errorf("relational intent requires a valid PostgreSQL schema snapshot")
	}
	if intent.Schema != RelationalIntentV1 {
		return fmt.Errorf("relational intent schema must be %q", RelationalIntentV1)
	}
	if intent.SourceID != snapshot.SourceID {
		return fmt.Errorf("relational intent source %q does not match snapshot source", intent.SourceID)
	}
	if intent.SchemaFingerprint != snapshot.Fingerprint {
		return fmt.Errorf("relational intent schema fingerprint is stale")
	}
	if _, err := snapshot.Relation(intent.FromRelationID); err != nil {
		return err
	}
	if intent.Limit <= 0 || intent.Limit > MaxIntentRows {
		return fmt.Errorf("relational intent limit must be within 1..%d", MaxIntentRows)
	}
	if len(intent.Projections) > MaxIntentProjections || len(intent.Filters) > MaxIntentFilters || len(intent.TemporalWindows) > MaxIntentFilters || len(intent.Exists) > MaxIntentExistenceChecks || len(intent.GroupBy) > MaxIntentGroups || len(intent.Having) > MaxIntentGroups || len(intent.OrderBy) > MaxIntentOrderTerms {
		return fmt.Errorf("relational intent exceeds a bounded collection limit")
	}
	for index, projection := range intent.Projections {
		if err := validateProjection(snapshot, projection); err != nil {
			return fmt.Errorf("projection %d: %w", index, err)
		}
	}
	for index, predicate := range intent.Filters {
		if err := validatePredicate(snapshot, predicate, ""); err != nil {
			return fmt.Errorf("filter %d: %w", index, err)
		}
	}
	for index, window := range intent.TemporalWindows {
		if err := validateTemporalWindow(snapshot, window); err != nil {
			return fmt.Errorf("temporal window %d: %w", index, err)
		}
	}
	for index, exists := range intent.Exists {
		if err := validateExistence(snapshot, exists); err != nil {
			return fmt.Errorf("existence predicate %d: %w", index, err)
		}
	}
	if err := validateGrouping(intent); err != nil {
		return err
	}
	for index, predicate := range intent.Having {
		if err := validateAggregatePredicate(snapshot, predicate); err != nil {
			return fmt.Errorf("having predicate %d: %w", index, err)
		}
	}
	if err := validateOrder(intent); err != nil {
		return err
	}
	return validateResultShape(intent)
}

func validatePredicate(snapshot SchemaSnapshot, predicate RelationalPredicate, requiredRelationID string) error {
	relation, column, err := snapshot.Column(predicate.FieldID)
	if err != nil {
		return err
	}
	if requiredRelationID != "" && relation.ID != requiredRelationID {
		return fmt.Errorf("field %q is outside existence relation %q", predicate.FieldID, requiredRelationID)
	}
	valueCount := len(predicate.Values)
	switch predicate.Operator {
	case FilterIsNull, FilterIsNotNull:
		if valueCount != 0 {
			return fmt.Errorf("operator %q accepts no values", predicate.Operator)
		}
		return nil
	case FilterIn, FilterNotIn:
		if valueCount == 0 || valueCount > MaxIntentFilterValues {
			return fmt.Errorf("operator %q requires 1..%d values", predicate.Operator, MaxIntentFilterValues)
		}
	case FilterEqual, FilterNotEqual, FilterGT, FilterGTE, FilterLT, FilterLTE, FilterContains, FilterPrefix:
		if valueCount != 1 {
			return fmt.Errorf("operator %q requires exactly one value", predicate.Operator)
		}
	default:
		return fmt.Errorf("unsupported filter operator %q", predicate.Operator)
	}
	if predicate.Operator == FilterContains || predicate.Operator == FilterPrefix {
		if column.TypeCategory != TypeText || len(column.AllowedValues) > 0 {
			return fmt.Errorf("operator %q requires a non-enum text field", predicate.Operator)
		}
	} else if predicate.Operator == FilterGT || predicate.Operator == FilterGTE || predicate.Operator == FilterLT || predicate.Operator == FilterLTE {
		switch column.TypeCategory {
		case TypeInteger, TypeDecimal, TypeTemporal, TypeDate:
		default:
			return fmt.Errorf("operator %q does not support field type %q", predicate.Operator, column.TypeCategory)
		}
	}
	seenValues := map[string]struct{}{}
	for _, value := range predicate.Values {
		key := string(value.Type) + "\x00" + value.Value
		if _, duplicate := seenValues[key]; duplicate {
			return fmt.Errorf("operator %q repeats one literal", predicate.Operator)
		}
		seenValues[key] = struct{}{}
		if err := validateLiteralForColumn(value, column); err != nil {
			return err
		}
	}
	return nil
}

func validateTemporalWindow(snapshot SchemaSnapshot, window TemporalWindow) error {
	_, column, err := snapshot.Column(window.FieldID)
	if err != nil {
		return err
	}
	if column.TypeCategory != TypeTemporal && column.TypeCategory != TypeDate {
		return fmt.Errorf("temporal window field must be temporal or date")
	}
	if window.Amount <= 0 || window.Amount > 10000 {
		return fmt.Errorf("temporal window amount must be within 1..10000")
	}
	switch window.Unit {
	case WindowHour:
		if column.TypeCategory == TypeDate {
			return fmt.Errorf("hour windows do not support date fields")
		}
	case WindowDay, WindowWeek, WindowMonth, WindowYear:
	default:
		return fmt.Errorf("unsupported temporal window unit %q", window.Unit)
	}
	if _, err := time.Parse(time.RFC3339, window.AsOf); err != nil {
		return fmt.Errorf("temporal window as_of must be RFC3339: %w", err)
	}
	return nil
}

func validateExistence(snapshot SchemaSnapshot, exists ExistencePredicate) error {
	if _, err := snapshot.Relation(exists.RelationID); err != nil {
		return err
	}
	if len(exists.Filters) > MaxIntentFilters {
		return fmt.Errorf("existence predicate exceeds %d filters", MaxIntentFilters)
	}
	for _, predicate := range exists.Filters {
		if err := validatePredicate(snapshot, predicate, exists.RelationID); err != nil {
			return err
		}
	}
	return nil
}

func validateGrouping(intent RelationalIntent) error {
	seen := map[int]struct{}{}
	for _, projectionIndex := range intent.GroupBy {
		if projectionIndex < 0 || projectionIndex >= len(intent.Projections) {
			return fmt.Errorf("group_by projection %d is out of range", projectionIndex)
		}
		projection := intent.Projections[projectionIndex]
		if projection.Aggregate != "" {
			return fmt.Errorf("group_by projection %d is an aggregate", projectionIndex)
		}
		if _, duplicate := seen[projectionIndex]; duplicate {
			return fmt.Errorf("group_by repeats projection %d", projectionIndex)
		}
		seen[projectionIndex] = struct{}{}
	}
	if len(intent.GroupBy) > 0 {
		for index, projection := range intent.Projections {
			if projection.Aggregate == "" {
				if _, grouped := seen[index]; !grouped {
					return fmt.Errorf("non-aggregate projection %d is not grouped", index)
				}
			}
		}
	}
	return nil
}

func validateAggregatePredicate(snapshot SchemaSnapshot, predicate AggregatePredicate) error {
	if err := validateAggregate(snapshot, predicate.Aggregate, predicate.FieldID); err != nil {
		return err
	}
	if predicate.Aggregate != AggregateCountRows && predicate.Aggregate != AggregateCount && predicate.Aggregate != AggregateCountDistinct && predicate.Aggregate != AggregateSum && predicate.Aggregate != AggregateAverage {
		return fmt.Errorf("having supports count, sum, and average aggregates only")
	}
	if predicate.Operator != FilterEqual && predicate.Operator != FilterNotEqual && predicate.Operator != FilterGT && predicate.Operator != FilterGTE && predicate.Operator != FilterLT && predicate.Operator != FilterLTE {
		return fmt.Errorf("having operator %q is unsupported", predicate.Operator)
	}
	if predicate.Value.Type != LiteralInteger && predicate.Value.Type != LiteralDecimal {
		return fmt.Errorf("having literal must be numeric")
	}
	return validateLiteral(predicate.Value)
}

func validateOrder(intent RelationalIntent) error {
	seen := map[int]struct{}{}
	for _, term := range intent.OrderBy {
		if term.Projection < 0 || term.Projection >= len(intent.Projections) {
			return fmt.Errorf("order projection %d is out of range", term.Projection)
		}
		if term.Direction != OrderAscending && term.Direction != OrderDescending {
			return fmt.Errorf("order direction %q is unsupported", term.Direction)
		}
		if _, duplicate := seen[term.Projection]; duplicate {
			return fmt.Errorf("order repeats projection %d", term.Projection)
		}
		seen[term.Projection] = struct{}{}
	}
	return nil
}

func validateResultShape(intent RelationalIntent) error {
	aggregates, buckets := 0, 0
	for _, projection := range intent.Projections {
		if projection.Aggregate != "" {
			aggregates++
		}
		if projection.TimeBucket != "" {
			buckets++
		}
	}
	switch intent.Shape {
	case ResultRecords:
		if len(intent.Projections) == 0 || aggregates != 0 || buckets != 0 || len(intent.GroupBy) != 0 || len(intent.Having) != 0 {
			return fmt.Errorf("records shape requires plain projections without grouping or aggregates")
		}
	case ResultScalar:
		if len(intent.Projections) != 1 || aggregates != 1 || len(intent.GroupBy) != 0 || len(intent.OrderBy) != 0 {
			return fmt.Errorf("scalar shape requires exactly one aggregate projection")
		}
	case ResultRanking:
		if aggregates == 0 || len(intent.GroupBy) == 0 || len(intent.OrderBy) == 0 {
			return fmt.Errorf("ranking shape requires grouping, an aggregate, and ordering")
		}
	case ResultDistribution, ResultComparison:
		if aggregates == 0 || len(intent.GroupBy) == 0 {
			return fmt.Errorf("shape %q requires grouping and an aggregate", intent.Shape)
		}
	case ResultTrend:
		if aggregates == 0 || buckets == 0 || len(intent.GroupBy) == 0 {
			return fmt.Errorf("trend shape requires a grouped time bucket and aggregate")
		}
	case ResultExistence:
		if len(intent.Projections) != 0 || len(intent.GroupBy) != 0 || len(intent.Having) != 0 || len(intent.OrderBy) != 0 {
			return fmt.Errorf("existence shape does not accept projections, grouping, having, or ordering")
		}
	default:
		return fmt.Errorf("unsupported result shape %q", intent.Shape)
	}
	if aggregates > 0 && intent.Shape != ResultScalar && len(intent.GroupBy) == 0 {
		return fmt.Errorf("aggregate projections require grouping outside scalar shape")
	}
	return nil
}

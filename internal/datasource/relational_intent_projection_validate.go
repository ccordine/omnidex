package datasource

import "fmt"

func validateProjection(snapshot SchemaSnapshot, projection RelationalProjection) error {
	forms := 0
	if projection.Aggregate != "" {
		forms++
	}
	if projection.TimeBucket != "" {
		forms++
	}
	if projection.Aggregate == "" && projection.TimeBucket == "" && projection.FieldID != "" {
		forms++
	}
	if forms != 1 {
		return fmt.Errorf("must express exactly one field, aggregate, or time bucket")
	}
	if projection.Aggregate != "" {
		return validateAggregate(snapshot, projection.Aggregate, projection.FieldID)
	}
	_, column, err := snapshot.Column(projection.FieldID)
	if err != nil {
		return err
	}
	if projection.TimeBucket != "" {
		if column.TypeCategory != TypeTemporal && column.TypeCategory != TypeDate {
			return fmt.Errorf("time bucket field must be temporal or date")
		}
		switch projection.TimeBucket {
		case BucketDay, BucketWeek, BucketMonth, BucketQuarter, BucketYear:
		default:
			return fmt.Errorf("unsupported time bucket %q", projection.TimeBucket)
		}
	}
	return nil
}

func validateAggregate(snapshot SchemaSnapshot, operation AggregateOperation, fieldID string) error {
	if operation == AggregateCountRows {
		if fieldID != "" {
			return fmt.Errorf("count_rows must not name a field")
		}
		return nil
	}
	if fieldID == "" {
		return fmt.Errorf("aggregate %q requires a field", operation)
	}
	_, column, err := snapshot.Column(fieldID)
	if err != nil {
		return err
	}
	switch operation {
	case AggregateCount, AggregateCountDistinct:
		return nil
	case AggregateSum, AggregateAverage:
		if column.TypeCategory != TypeInteger && column.TypeCategory != TypeDecimal {
			return fmt.Errorf("aggregate %q requires a numeric field", operation)
		}
	case AggregateMinimum, AggregateMaximum:
		if len(column.AllowedValues) > 0 {
			return fmt.Errorf("aggregate %q does not support enum fields", operation)
		}
		switch column.TypeCategory {
		case TypeInteger, TypeDecimal, TypeText, TypeTemporal, TypeDate:
		default:
			return fmt.Errorf("aggregate %q does not support field type %q", operation, column.TypeCategory)
		}
	default:
		return fmt.Errorf("unsupported aggregate %q", operation)
	}
	return nil
}

package assemblyline

import (
	"fmt"

	"github.com/gryph/omnidex/internal/datasource"
)

func databaseQueryShapeDescription(shape datasource.ResultShape) (string, error) {
	descriptions := map[datasource.ResultShape]string{
		datasource.ResultRecords:      "matching records with requested fields",
		datasource.ResultScalar:       "one aggregate value",
		datasource.ResultRanking:      "records ordered by a requested measure",
		datasource.ResultDistribution: "values grouped into a distribution",
		datasource.ResultComparison:   "requested values compared across groups",
		datasource.ResultTrend:        "requested values over time",
		datasource.ResultExistence:    "whether matching records exist",
	}
	description, ok := descriptions[shape]
	if !ok {
		return "", fmt.Errorf("database query result shape %q has no semantic description", shape)
	}
	return description, nil
}

func databaseQueryAggregateDescription(operation datasource.AggregateOperation) (string, error) {
	descriptions := map[datasource.AggregateOperation]string{
		datasource.AggregateCountRows:     "count matching rows",
		datasource.AggregateCount:         "count non-null field values",
		datasource.AggregateCountDistinct: "count distinct non-null field values",
		datasource.AggregateSum:           "sum numeric field values",
		datasource.AggregateAverage:       "average numeric field values",
		datasource.AggregateMinimum:       "minimum field value",
		datasource.AggregateMaximum:       "maximum field value",
	}
	description, ok := descriptions[operation]
	if !ok {
		return "", fmt.Errorf("database query aggregate %q has no semantic description", operation)
	}
	return description, nil
}

func databaseQueryTimeBucketDescription(bucket datasource.TimeBucketUnit) (string, error) {
	descriptions := map[datasource.TimeBucketUnit]string{
		datasource.BucketDay:     "calendar day",
		datasource.BucketWeek:    "calendar week",
		datasource.BucketMonth:   "calendar month",
		datasource.BucketQuarter: "calendar quarter",
		datasource.BucketYear:    "calendar year",
	}
	description, ok := descriptions[bucket]
	if !ok {
		return "", fmt.Errorf("database query time bucket %q has no semantic description", bucket)
	}
	return description, nil
}

func databaseQueryFilterOperatorDescription(operator datasource.FilterOperator) (string, error) {
	descriptions := map[datasource.FilterOperator]string{
		datasource.FilterEqual:     "Equal to the requested value",
		datasource.FilterNotEqual:  "Not equal to the requested value",
		datasource.FilterGT:        "Greater than the requested value",
		datasource.FilterGTE:       "Greater than or equal to the requested value",
		datasource.FilterLT:        "Less than the requested value",
		datasource.FilterLTE:       "Less than or equal to the requested value",
		datasource.FilterIn:        "Equal to any requested value",
		datasource.FilterNotIn:     "Equal to none of the requested values",
		datasource.FilterIsNull:    "Has no value",
		datasource.FilterIsNotNull: "Has a value",
		datasource.FilterContains:  "Contains the requested text",
		datasource.FilterPrefix:    "Starts with the requested text",
	}
	description, ok := descriptions[operator]
	if !ok {
		return "", fmt.Errorf("database query filter operator %q has no semantic description", operator)
	}
	return description, nil
}

func databaseQueryWindowUnitDescription(unit datasource.WindowUnit) (string, error) {
	descriptions := map[datasource.WindowUnit]string{
		datasource.WindowHour:  "hours",
		datasource.WindowDay:   "days",
		datasource.WindowWeek:  "weeks",
		datasource.WindowMonth: "months",
		datasource.WindowYear:  "years",
	}
	description, ok := descriptions[unit]
	if !ok {
		return "", fmt.Errorf("database query window unit %q has no semantic description", unit)
	}
	return description, nil
}

func databaseQueryOrderDirectionDescription(direction datasource.OrderDirection) (string, error) {
	switch direction {
	case datasource.OrderAscending:
		return "ascending, with smaller or earlier values first", nil
	case datasource.OrderDescending:
		return "descending, with larger or later values first", nil
	default:
		return "", fmt.Errorf("database query order direction %q has no semantic description", direction)
	}
}

package assemblyline

import (
	"fmt"

	"github.com/gryph/omnidex/internal/datasource"
)

func renderDatabaseQueryFocusedProjection(
	state DatabaseQueryIntentLeafState,
	index int,
) (string, error) {
	if index < 0 || index >= len(state.Projections) {
		return "", fmt.Errorf("database query focused projection is outside accepted projections")
	}
	semantic, err := databaseQueryProjectionSemantic(state, state.Projections[index])
	if err != nil {
		return "", err
	}
	return "FOCUSED PROJECTION:\n" + semantic, nil
}

func databaseQueryProjectionSemantic(
	state DatabaseQueryIntentLeafState,
	projection datasource.RelationalProjection,
) (string, error) {
	if projection.Aggregate == datasource.AggregateCountRows {
		return "count matching rows", nil
	}
	field, err := databaseQueryFieldSemantic(state, projection.FieldID)
	if err != nil {
		return "", err
	}
	if projection.Aggregate != "" {
		aggregate, err := databaseQueryAggregateDescription(projection.Aggregate)
		if err != nil {
			return "", err
		}
		return aggregate + " for " + field, nil
	}
	if projection.TimeBucket != "" {
		bucket, err := databaseQueryTimeBucketDescription(projection.TimeBucket)
		if err != nil {
			return "", err
		}
		return field + " grouped by " + bucket, nil
	}
	return field, nil
}

func renderDatabaseQueryFilterOperator(input DatabaseQueryFilterLeafInput) (string, error) {
	if input.Operator == "" {
		return "", nil
	}
	description, err := databaseQueryFilterOperatorDescription(input.Operator)
	if err != nil {
		return "", err
	}
	return "ACCEPTED FILTER RELATION:\n" + description, nil
}

func databaseQueryFieldSemantic(state DatabaseQueryIntentLeafState, fieldID string) (string, error) {
	column, relationID, ok := databaseQueryColumn(state, fieldID)
	if !ok {
		return "", fmt.Errorf("database query semantic field %q was not projected", fieldID)
	}
	relation, ok := databaseQueryProjectedRelation(state, relationID)
	if !ok {
		return "", fmt.Errorf("database query semantic field relation %q was not projected", relationID)
	}
	return relation.SchemaName + "." + relation.Name + "." + column.Name, nil
}

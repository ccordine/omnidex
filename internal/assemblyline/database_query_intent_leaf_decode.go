package assemblyline

import (
	"fmt"

	"github.com/gryph/omnidex/internal/datasource"
)

func DecodeDatabaseQueryFromRelationLeaf(
	state DatabaseQueryIntentLeafState,
	raw string,
) (string, error) {
	if err := state.validate(); err != nil {
		return "", err
	}
	leaf, err := decodeDatabaseQueryRawLeaf("database query from relation", raw)
	if err != nil {
		return "", err
	}
	if !databaseQueryRelationExists(state, leaf) {
		return "", fmt.Errorf("database query from relation ID %q was not projected", leaf)
	}
	return leaf, nil
}

func DecodeDatabaseQueryShapeLeaf(
	state DatabaseQueryIntentLeafState,
	raw string,
) (datasource.ResultShape, error) {
	if err := state.validate(); err != nil {
		return "", err
	}
	leaf, err := decodeDatabaseQueryRawLeaf("database query shape", raw)
	if err != nil {
		return "", err
	}
	shape := datasource.ResultShape(leaf)
	if !validDatabaseQueryShape(shape) {
		return "", fmt.Errorf("database query shape %q is not registered", shape)
	}
	return shape, nil
}

func DecodeDatabaseQueryProjectionCoverageLeaf(
	state DatabaseQueryIntentLeafState,
	raw string,
) (string, error) {
	if err := state.validateReady(); err != nil {
		return "", err
	}
	return decodeDatabaseQueryCollectionCoverage(
		"database query projection coverage", raw,
		DatabaseQueryProjectionsHaveRequiredShape(state),
		len(state.Projections) == datasource.MaxIntentProjections,
	)
}

func DecodeDatabaseQueryProjectionAggregateLeaf(
	input DatabaseQueryProjectionLeafInput,
	raw string,
) (datasource.AggregateOperation, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	leaf, err := decodeDatabaseQueryRawLeaf("database query projection aggregate", raw)
	if err != nil {
		return "", err
	}
	if leaf == "none" {
		if input.State.Shape == datasource.ResultScalar {
			return "", fmt.Errorf("database query scalar projection requires one aggregate")
		}
		return "", nil
	}
	operation := datasource.AggregateOperation(leaf)
	if !validDatabaseQueryProjectionAggregate(operation) {
		return "", fmt.Errorf("database query projection aggregate %q is not registered", leaf)
	}
	return operation, nil
}

func DecodeDatabaseQueryProjectionFieldLeaf(
	input DatabaseQueryProjectionLeafInput,
	raw string,
) (string, error) {
	if err := input.validateForField(); err != nil {
		return "", err
	}
	leaf, err := decodeDatabaseQueryRawLeaf("database query projection field", raw)
	if err != nil {
		return "", err
	}
	if _, _, ok := databaseQueryColumn(input.State, leaf); !ok {
		return "", fmt.Errorf("database query projection field ID %q was not projected", leaf)
	}
	projection := datasource.RelationalProjection{FieldID: leaf, Aggregate: input.Aggregate}
	if err := validateDatabaseQueryProjection(input.State, projection); err != nil {
		return "", err
	}
	return leaf, nil
}

func DecodeDatabaseQueryProjectionTimeBucketLeaf(
	input DatabaseQueryProjectionLeafInput,
	raw string,
) (datasource.TimeBucketUnit, error) {
	if err := input.validateForTimeBucket(); err != nil {
		return "", err
	}
	leaf, err := decodeDatabaseQueryRawLeaf("database query projection time bucket", raw)
	if err != nil {
		return "", err
	}
	var bucket datasource.TimeBucketUnit
	if leaf != "none" {
		bucket = datasource.TimeBucketUnit(leaf)
		switch bucket {
		case datasource.BucketDay, datasource.BucketWeek, datasource.BucketMonth,
			datasource.BucketQuarter, datasource.BucketYear:
		default:
			return "", fmt.Errorf("database query time bucket %q is not registered", leaf)
		}
	}
	projection := datasource.RelationalProjection{FieldID: input.FieldID, TimeBucket: bucket}
	if err := validateDatabaseQueryProjection(input.State, projection); err != nil {
		return "", err
	}
	return bucket, nil
}

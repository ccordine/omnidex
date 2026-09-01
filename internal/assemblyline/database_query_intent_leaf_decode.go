package assemblyline

import "github.com/gryph/omnidex/internal/datasource"

func DecodeDatabaseQueryFromRelationLeaf(
	state DatabaseQueryIntentLeafState,
	raw string,
) (string, error) {
	if err := state.validate(); err != nil {
		return "", err
	}
	choices, err := databaseQueryFromRelationChoices(state)
	if err != nil {
		return "", err
	}
	return DecodeOpaqueModelChoice(raw, choices)
}

func DecodeDatabaseQueryShapeLeaf(
	state DatabaseQueryIntentLeafState,
	raw string,
) (datasource.ResultShape, error) {
	if err := state.validate(); err != nil {
		return "", err
	}
	choices, err := databaseQueryShapeChoices()
	if err != nil {
		return "", err
	}
	value, err := DecodeOpaqueModelChoice(raw, choices)
	if err != nil {
		return "", err
	}
	return datasource.ResultShape(value), nil
}

func DecodeDatabaseQueryProjectionAggregateLeaf(
	input DatabaseQueryProjectionLeafInput,
	raw string,
) (datasource.AggregateOperation, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	choices, err := databaseQueryProjectionAggregateChoices(input)
	if err != nil {
		return "", err
	}
	value, err := DecodeOpaqueModelChoice(raw, choices)
	if err != nil {
		return "", err
	}
	if value == databaseQueryDirectProjectionChoice {
		return "", nil
	}
	return datasource.AggregateOperation(value), nil
}

func DecodeDatabaseQueryProjectionFieldLeaf(
	input DatabaseQueryProjectionLeafInput,
	raw string,
) (string, error) {
	if err := input.validateForField(); err != nil {
		return "", err
	}
	choices, err := databaseQueryFieldChoices(
		input.State, "", databaseQueryAggregateFieldEligible(input.Aggregate),
	)
	if err != nil {
		return "", err
	}
	fieldID, err := DecodeOpaqueModelChoice(raw, choices)
	if err != nil {
		return "", err
	}
	projection := datasource.RelationalProjection{FieldID: fieldID, Aggregate: input.Aggregate}
	if err := validateDatabaseQueryProjection(input.State, projection); err != nil {
		return "", err
	}
	return fieldID, nil
}

func DecodeDatabaseQueryProjectionTimeBucketLeaf(
	input DatabaseQueryProjectionLeafInput,
	raw string,
) (datasource.TimeBucketUnit, error) {
	if err := input.validateForTimeBucket(); err != nil {
		return "", err
	}
	choices, err := databaseQueryProjectionTimeBucketChoices()
	if err != nil {
		return "", err
	}
	value, err := DecodeOpaqueModelChoice(raw, choices)
	if err != nil {
		return "", err
	}
	var bucket datasource.TimeBucketUnit
	if value != databaseQueryNoTimeBucketChoice {
		bucket = datasource.TimeBucketUnit(value)
	}
	projection := datasource.RelationalProjection{FieldID: input.FieldID, TimeBucket: bucket}
	if err := validateDatabaseQueryProjection(input.State, projection); err != nil {
		return "", err
	}
	return bucket, nil
}

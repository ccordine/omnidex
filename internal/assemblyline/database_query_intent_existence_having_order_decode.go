package assemblyline

import (
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/datasource"
)

func DecodeDatabaseQueryExistenceRelationLeaf(
	input DatabaseQueryExistenceLeafInput,
	raw string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	excluded := make(map[string]struct{}, len(input.State.Exists))
	for _, accepted := range input.State.Exists {
		excluded[accepted.RelationID] = struct{}{}
	}
	choices, err := databaseQueryRelationChoicesExcluding(input.State, excluded)
	if err != nil {
		return "", err
	}
	return DecodeOpaqueModelChoice(raw, choices)
}

func DecodeDatabaseQueryExistenceNegatedLeaf(
	input DatabaseQueryExistenceLeafInput,
	raw string,
) (bool, error) {
	if err := input.validateRelation(); err != nil {
		return false, err
	}
	choices, err := databaseQueryExistenceNegatedChoices()
	if err != nil {
		return false, err
	}
	value, err := DecodeOpaqueModelChoice(raw, choices)
	if err != nil {
		return false, err
	}
	return value == "true", nil
}

func DecodeDatabaseQueryHavingAggregateLeaf(
	input DatabaseQueryHavingLeafInput,
	raw string,
) (datasource.AggregateOperation, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	choices, err := databaseQueryHavingAggregateChoices()
	if err != nil {
		return "", err
	}
	value, err := DecodeOpaqueModelChoice(raw, choices)
	if err != nil {
		return "", err
	}
	return datasource.AggregateOperation(value), nil
}

func DecodeDatabaseQueryHavingFieldLeaf(
	input DatabaseQueryHavingLeafInput,
	raw string,
) (string, error) {
	if err := input.validateAggregate(); err != nil {
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

func DecodeDatabaseQueryHavingOperatorLeaf(
	input DatabaseQueryHavingLeafInput,
	raw string,
) (datasource.FilterOperator, error) {
	if err := input.validateField(); err != nil {
		return "", err
	}
	choices, err := databaseQueryHavingOperatorChoices()
	if err != nil {
		return "", err
	}
	value, err := DecodeOpaqueModelChoice(raw, choices)
	if err != nil {
		return "", err
	}
	return datasource.FilterOperator(value), nil
}

func DecodeDatabaseQueryHavingValueLeaf(
	input DatabaseQueryHavingLeafInput,
	raw string,
) (datasource.IntentLiteral, error) {
	if err := input.validateOperator(); err != nil {
		return datasource.IntentLiteral{}, err
	}
	leaf, err := decodeDatabaseQueryRawLeaf("database query having value", raw)
	if err != nil {
		return datasource.IntentLiteral{}, err
	}
	literalType := datasource.LiteralInteger
	if strings.Contains(leaf, ".") {
		literalType = datasource.LiteralDecimal
	}
	literal := datasource.IntentLiteral{Type: literalType, Value: leaf}
	predicate := datasource.AggregatePredicate{
		Aggregate: input.Aggregate, FieldID: input.FieldID,
		Operator: input.Operator, Value: literal,
	}
	if err := validateDatabaseQueryHaving(input.State, predicate); err != nil {
		return datasource.IntentLiteral{}, err
	}
	return literal, nil
}

func DecodeDatabaseQueryOrderProjectionLeaf(
	input DatabaseQueryOrderLeafInput,
	raw string,
) (int, error) {
	if err := input.validate(); err != nil {
		return 0, err
	}
	choices, err := databaseQueryOrderProjectionChoices(input)
	if err != nil {
		return 0, err
	}
	value, err := DecodeOpaqueModelChoice(raw, choices)
	if err != nil {
		return 0, err
	}
	projection, _ := strconv.Atoi(value)
	return projection, nil
}

func DecodeDatabaseQueryOrderDirectionLeaf(
	input DatabaseQueryOrderLeafInput,
	raw string,
) (datasource.OrderDirection, error) {
	if err := input.validateProjection(); err != nil {
		return "", err
	}
	choices, err := databaseQueryOrderDirectionChoices()
	if err != nil {
		return "", err
	}
	value, err := DecodeOpaqueModelChoice(raw, choices)
	if err != nil {
		return "", err
	}
	return datasource.OrderDirection(value), nil
}

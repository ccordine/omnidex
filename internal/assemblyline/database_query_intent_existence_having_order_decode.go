package assemblyline

import (
	"fmt"
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
	leaf, err := decodeDatabaseQueryRawLeaf("database query existence relation", raw)
	if err != nil {
		return "", err
	}
	if !databaseQueryRelationExists(input.State, leaf) {
		return "", fmt.Errorf("database query existence relation %q was not projected", leaf)
	}
	for _, accepted := range input.State.Exists {
		if accepted.RelationID == leaf {
			return "", fmt.Errorf("database query existence relation %q is already used", leaf)
		}
	}
	return leaf, nil
}

func DecodeDatabaseQueryExistenceNegatedLeaf(
	input DatabaseQueryExistenceLeafInput,
	raw string,
) (bool, error) {
	if err := input.validateRelation(); err != nil {
		return false, err
	}
	leaf, err := decodeDatabaseQueryRawLeaf("database query existence negation", raw)
	if err != nil {
		return false, err
	}
	switch leaf {
	case "EXISTS":
		return false, nil
	case "NOT_EXISTS":
		return true, nil
	default:
		return false, fmt.Errorf("database query existence value %q is not registered", leaf)
	}
}

func DecodeDatabaseQueryHavingAggregateLeaf(
	input DatabaseQueryHavingLeafInput,
	raw string,
) (datasource.AggregateOperation, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	leaf, err := decodeDatabaseQueryRawLeaf("database query having aggregate", raw)
	if err != nil {
		return "", err
	}
	aggregate := datasource.AggregateOperation(leaf)
	if !validDatabaseQueryHavingAggregate(aggregate) {
		return "", fmt.Errorf("database query having aggregate %q is not registered", aggregate)
	}
	return aggregate, nil
}

func DecodeDatabaseQueryHavingFieldLeaf(
	input DatabaseQueryHavingLeafInput,
	raw string,
) (string, error) {
	if err := input.validateAggregate(); err != nil {
		return "", err
	}
	leaf, err := decodeDatabaseQueryRawLeaf("database query having field", raw)
	if err != nil {
		return "", err
	}
	if _, _, ok := databaseQueryColumn(input.State, leaf); !ok {
		return "", fmt.Errorf("database query having field %q was not projected", leaf)
	}
	projection := datasource.RelationalProjection{FieldID: leaf, Aggregate: input.Aggregate}
	if err := validateDatabaseQueryProjection(input.State, projection); err != nil {
		return "", err
	}
	return leaf, nil
}

func DecodeDatabaseQueryHavingOperatorLeaf(
	input DatabaseQueryHavingLeafInput,
	raw string,
) (datasource.FilterOperator, error) {
	if err := input.validateField(); err != nil {
		return "", err
	}
	leaf, err := decodeDatabaseQueryRawLeaf("database query having operator", raw)
	if err != nil {
		return "", err
	}
	operator := datasource.FilterOperator(leaf)
	switch operator {
	case datasource.FilterEqual, datasource.FilterNotEqual, datasource.FilterGT,
		datasource.FilterGTE, datasource.FilterLT, datasource.FilterLTE:
		return operator, nil
	default:
		return "", fmt.Errorf("database query having operator %q is not registered", operator)
	}
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
	leaf, err := decodeDatabaseQueryRawLeaf("database query order projection", raw)
	if err != nil {
		return 0, err
	}
	projection, err := strconv.Atoi(leaf)
	if err != nil || strconv.Itoa(projection) != leaf || projection < 0 || projection >= len(input.State.Projections) {
		return 0, fmt.Errorf("database query order projection %q is outside accepted projections", leaf)
	}
	for _, accepted := range input.State.OrderBy {
		if accepted.Projection == projection {
			return 0, fmt.Errorf("database query order projection %d is already used", projection)
		}
	}
	return projection, nil
}

func DecodeDatabaseQueryOrderDirectionLeaf(
	input DatabaseQueryOrderLeafInput,
	raw string,
) (datasource.OrderDirection, error) {
	if err := input.validateProjection(); err != nil {
		return "", err
	}
	leaf, err := decodeDatabaseQueryRawLeaf("database query order direction", raw)
	if err != nil {
		return "", err
	}
	direction := datasource.OrderDirection(leaf)
	if direction != datasource.OrderAscending && direction != datasource.OrderDescending {
		return "", fmt.Errorf("database query order direction %q is not registered", direction)
	}
	return direction, nil
}

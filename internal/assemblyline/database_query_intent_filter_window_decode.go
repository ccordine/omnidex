package assemblyline

import (
	"fmt"
	"strconv"

	"github.com/gryph/omnidex/internal/datasource"
)

func DecodeDatabaseQueryFilterFieldLeaf(
	input DatabaseQueryFilterLeafInput,
	raw string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	choices, err := databaseQueryFieldChoices(input.State, input.ScopeRelationID, nil)
	if err != nil {
		return "", err
	}
	return DecodeOpaqueModelChoice(raw, choices)
}

func DecodeDatabaseQueryFilterOperatorLeaf(
	input DatabaseQueryFilterLeafInput,
	raw string,
) (datasource.FilterOperator, error) {
	if err := input.validateField(); err != nil {
		return "", err
	}
	choices, err := databaseQueryFilterOperatorChoices(input)
	if err != nil {
		return "", err
	}
	value, err := DecodeOpaqueModelChoice(raw, choices)
	if err != nil {
		return "", err
	}
	return datasource.FilterOperator(value), nil
}

func DecodeDatabaseQueryFilterValueLeaf(
	input DatabaseQueryFilterLeafInput,
	raw string,
) (datasource.IntentLiteral, error) {
	if err := input.validateOperator(); err != nil {
		return datasource.IntentLiteral{}, err
	}
	choices, closed, err := databaseQueryFilterValueChoices(input)
	if err != nil {
		return datasource.IntentLiteral{}, err
	}
	var value string
	if closed {
		value, err = DecodeOpaqueModelChoice(raw, choices)
	} else {
		value, err = decodeRawSemanticLeaf(
			"database query filter value", raw, maxDatabaseQueryLeafBytes, false,
		)
	}
	if err != nil {
		return datasource.IntentLiteral{}, err
	}
	return databaseQueryFilterValue(input, value)
}

func databaseQueryFilterValue(
	input DatabaseQueryFilterLeafInput,
	value string,
) (datasource.IntentLiteral, error) {
	literal, err := databaseQueryLiteral(input.State, input.FieldID, value)
	if err != nil {
		return datasource.IntentLiteral{}, err
	}
	for _, accepted := range input.AcceptedValues {
		if accepted == literal {
			return datasource.IntentLiteral{}, fmt.Errorf("database query filter value is duplicated")
		}
	}
	predicate := datasource.RelationalPredicate{
		FieldID: input.FieldID, Operator: input.Operator,
		Values: append(append([]datasource.IntentLiteral{}, input.AcceptedValues...), literal),
	}
	if err := validateDatabaseQueryPredicate(input.State, predicate, input.ScopeRelationID); err != nil {
		return datasource.IntentLiteral{}, err
	}
	return literal, nil
}

func DecodeDatabaseQueryWindowFieldLeaf(
	input DatabaseQueryWindowLeafInput,
	raw string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	choices, err := databaseQueryFieldChoices(input.State, "", databaseQueryTemporalFieldEligible)
	if err != nil {
		return "", err
	}
	return DecodeOpaqueModelChoice(raw, choices)
}

func DecodeDatabaseQueryWindowUnitLeaf(
	input DatabaseQueryWindowLeafInput,
	raw string,
) (datasource.WindowUnit, error) {
	if err := input.validateField(); err != nil {
		return "", err
	}
	choices, err := databaseQueryWindowUnitChoices(input)
	if err != nil {
		return "", err
	}
	value, err := DecodeOpaqueModelChoice(raw, choices)
	if err != nil {
		return "", err
	}
	return datasource.WindowUnit(value), nil
}

func DecodeDatabaseQueryWindowAmountLeaf(
	input DatabaseQueryWindowLeafInput,
	raw string,
) (int, error) {
	if err := input.validateUnit(); err != nil {
		return 0, err
	}
	leaf, err := decodeDatabaseQueryRawLeaf("database query window amount", raw)
	if err != nil {
		return 0, err
	}
	amount, err := strconv.Atoi(leaf)
	if err != nil || strconv.Itoa(amount) != leaf || amount < 1 || amount > 10000 {
		return 0, fmt.Errorf("database query window amount must be canonical within 1..10000")
	}
	window := DatabaseTemporalWindowDecision{FieldID: input.FieldID, Unit: input.Unit, Amount: amount}
	if err := validateDatabaseQueryWindow(input.State, window); err != nil {
		return 0, err
	}
	return amount, nil
}

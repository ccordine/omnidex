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
	leaf, err := decodeDatabaseQueryRawLeaf("database query filter field", raw)
	if err != nil {
		return "", err
	}
	_, relationID, ok := databaseQueryColumn(input.State, leaf)
	if !ok || input.ScopeRelationID != "" && relationID != input.ScopeRelationID {
		return "", fmt.Errorf("database query filter field ID %q is outside its authority", leaf)
	}
	return leaf, nil
}

func DecodeDatabaseQueryFilterOperatorLeaf(
	input DatabaseQueryFilterLeafInput,
	raw string,
) (datasource.FilterOperator, error) {
	if err := input.validateField(); err != nil {
		return "", err
	}
	leaf, err := decodeDatabaseQueryRawLeaf("database query filter operator", raw)
	if err != nil {
		return "", err
	}
	operator := datasource.FilterOperator(leaf)
	for _, allowed := range databaseQueryFilterOperators(input.State, input.FieldID) {
		if operator == allowed {
			return operator, nil
		}
	}
	return "", fmt.Errorf("database query filter operator %q is invalid for field %q", operator, input.FieldID)
}

func DecodeDatabaseQueryFilterValueLeaf(
	input DatabaseQueryFilterLeafInput,
	raw string,
) (datasource.IntentLiteral, error) {
	if err := input.validateOperator(); err != nil {
		return datasource.IntentLiteral{}, err
	}
	leaf, err := decodeRawSemanticLeaf(
		"database query filter value", raw, maxDatabaseQueryLeafBytes, false,
	)
	if err != nil {
		return datasource.IntentLiteral{}, err
	}
	literal, err := databaseQueryLiteral(input.State, input.FieldID, leaf)
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
	leaf, err := decodeDatabaseQueryRawLeaf("database query window field", raw)
	if err != nil {
		return "", err
	}
	column, _, ok := databaseQueryColumn(input.State, leaf)
	if !ok || column.TypeCategory != datasource.TypeTemporal && column.TypeCategory != datasource.TypeDate {
		return "", fmt.Errorf("database query window field %q is not temporal", leaf)
	}
	return leaf, nil
}

func DecodeDatabaseQueryWindowUnitLeaf(
	input DatabaseQueryWindowLeafInput,
	raw string,
) (datasource.WindowUnit, error) {
	if err := input.validateField(); err != nil {
		return "", err
	}
	leaf, err := decodeDatabaseQueryRawLeaf("database query window unit", raw)
	if err != nil {
		return "", err
	}
	unit := datasource.WindowUnit(leaf)
	if !validDatabaseQueryWindowUnit(unit) {
		return "", fmt.Errorf("database query window unit %q is not registered", unit)
	}
	column, _, _ := databaseQueryColumn(input.State, input.FieldID)
	if unit == datasource.WindowHour && column.TypeCategory == datasource.TypeDate {
		return "", fmt.Errorf("database query hour window does not support date field %q", input.FieldID)
	}
	return unit, nil
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

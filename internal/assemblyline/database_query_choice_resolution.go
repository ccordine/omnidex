package assemblyline

import (
	"fmt"
	"strconv"

	"github.com/gryph/omnidex/internal/datasource"
)

func ResolveSoleDatabaseQueryFromRelationLeaf(
	state DatabaseQueryIntentLeafState,
) (string, bool, error) {
	choices, err := databaseQueryFromRelationChoices(state)
	if err != nil {
		return "", false, err
	}
	if len(choices) == 0 {
		return "", false, fmt.Errorf("database query projection has no compatible field")
	}
	return resolveSoleDatabaseOpaqueChoice(choices)
}

func ResolveSoleDatabaseQueryProjectionFieldLeaf(
	input DatabaseQueryProjectionLeafInput,
) (string, bool, error) {
	if err := input.validateForField(); err != nil {
		return "", false, err
	}
	choices, err := databaseQueryFieldChoices(
		input.State, "", databaseQueryAggregateFieldEligible(input.Aggregate),
	)
	if err != nil {
		return "", false, err
	}
	if len(choices) == 0 {
		return "", false, fmt.Errorf("database query filter has no compatible field")
	}
	return resolveSoleDatabaseOpaqueChoice(choices)
}

func ResolveSoleDatabaseQueryFilterFieldLeaf(
	input DatabaseQueryFilterLeafInput,
) (string, bool, error) {
	if err := input.validate(); err != nil {
		return "", false, err
	}
	choices, err := databaseQueryFieldChoices(input.State, input.ScopeRelationID, nil)
	if err != nil {
		return "", false, err
	}
	return resolveSoleDatabaseOpaqueChoice(choices)
}

func ResolveSoleDatabaseQueryFilterOperatorLeaf(
	input DatabaseQueryFilterLeafInput,
) (datasource.FilterOperator, bool, error) {
	choices, err := databaseQueryFilterOperatorChoices(input)
	if err != nil {
		return "", false, err
	}
	value, resolved, err := resolveSoleDatabaseOpaqueChoice(choices)
	return datasource.FilterOperator(value), resolved, err
}

func ResolveSoleDatabaseQueryFilterValueLeaf(
	input DatabaseQueryFilterLeafInput,
) (datasource.IntentLiteral, bool, error) {
	choices, closed, err := databaseQueryFilterValueChoices(input)
	if err != nil || !closed {
		return datasource.IntentLiteral{}, false, err
	}
	if len(choices) == 0 {
		return datasource.IntentLiteral{}, false, fmt.Errorf(
			"database query filter has no unused available value",
		)
	}
	value, resolved, err := resolveSoleDatabaseOpaqueChoice(choices)
	if err != nil || !resolved {
		return datasource.IntentLiteral{}, resolved, err
	}
	literal, err := databaseQueryFilterValue(input, value)
	if err != nil {
		return datasource.IntentLiteral{}, false, err
	}
	return literal, true, nil
}

func ResolveSoleDatabaseQueryWindowFieldLeaf(
	input DatabaseQueryWindowLeafInput,
) (string, bool, error) {
	if err := input.validate(); err != nil {
		return "", false, err
	}
	choices, err := databaseQueryFieldChoices(
		input.State, "", databaseQueryTemporalFieldEligible,
	)
	if err != nil {
		return "", false, err
	}
	if len(choices) == 0 {
		return "", false, fmt.Errorf("database query window has no temporal field")
	}
	return resolveSoleDatabaseOpaqueChoice(choices)
}

func ResolveSoleDatabaseQueryWindowUnitLeaf(
	input DatabaseQueryWindowLeafInput,
) (datasource.WindowUnit, bool, error) {
	choices, err := databaseQueryWindowUnitChoices(input)
	if err != nil {
		return "", false, err
	}
	value, resolved, err := resolveSoleDatabaseOpaqueChoice(choices)
	return datasource.WindowUnit(value), resolved, err
}

func ResolveSoleDatabaseQueryExistenceRelationLeaf(
	input DatabaseQueryExistenceLeafInput,
) (string, bool, error) {
	if err := input.validate(); err != nil {
		return "", false, err
	}
	excluded := make(map[string]struct{}, len(input.State.Exists))
	for _, accepted := range input.State.Exists {
		excluded[accepted.RelationID] = struct{}{}
	}
	choices, err := databaseQueryRelationChoicesExcluding(input.State, excluded)
	if err != nil {
		return "", false, err
	}
	if len(choices) == 0 {
		return "", false, fmt.Errorf(
			"database query existence has no unused projected relation",
		)
	}
	return resolveSoleDatabaseOpaqueChoice(choices)
}

func ResolveSoleDatabaseQueryHavingFieldLeaf(
	input DatabaseQueryHavingLeafInput,
) (string, bool, error) {
	if err := input.validateAggregate(); err != nil {
		return "", false, err
	}
	choices, err := databaseQueryFieldChoices(
		input.State, "", databaseQueryAggregateFieldEligible(input.Aggregate),
	)
	if err != nil {
		return "", false, err
	}
	if len(choices) == 0 {
		return "", false, fmt.Errorf(
			"database query having aggregate has no compatible field",
		)
	}
	return resolveSoleDatabaseOpaqueChoice(choices)
}

func ResolveSoleDatabaseQueryOrderProjectionLeaf(
	input DatabaseQueryOrderLeafInput,
) (int, bool, error) {
	choices, err := databaseQueryOrderProjectionChoices(input)
	if err != nil {
		return 0, false, err
	}
	if len(choices) == 0 {
		return 0, false, fmt.Errorf("database query order has no unused projection")
	}
	value, resolved, err := resolveSoleDatabaseOpaqueChoice(choices)
	if err != nil || !resolved {
		return 0, resolved, err
	}
	projection, err := strconv.Atoi(value)
	if err != nil {
		return 0, false, err
	}
	return projection, true, nil
}

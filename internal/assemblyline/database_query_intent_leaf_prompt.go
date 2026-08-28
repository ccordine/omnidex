package assemblyline

import (
	"fmt"
)

func BuildDatabaseQueryFromRelationPrompt(state DatabaseQueryIntentLeafState) (string, error) {
	if err := state.validate(); err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		"Select the one opaque relation ID that should anchor the relational query for the exact evidence need.",
		"Return exactly one projected relation ID. Return no JSON, quotes, label, SQL, explanation, or commentary.",
		renderDatabaseQueryAuthority(state, true, ""),
	), nil
}

func BuildDatabaseQueryShapePrompt(state DatabaseQueryIntentLeafState) (string, error) {
	if err := state.validate(); err != nil {
		return "", err
	}
	if state.FromRelationID == "" {
		return "", fmt.Errorf("database query shape requires an accepted from relation")
	}
	return databaseQueryLeafPrompt(
		"Select the one result shape that directly answers the exact evidence need.",
		"Return exactly one registered value: records, scalar, ranking, distribution, comparison, trend, or existence. Return no JSON, quotes, label, SQL, explanation, or commentary.",
		renderDatabaseQueryAuthority(state, false, ""),
	), nil
}

func BuildDatabaseQueryProjectionCoveragePrompt(state DatabaseQueryIntentLeafState) (string, error) {
	if err := state.validateReady(); err != nil {
		return "", err
	}
	return databaseQueryCoveragePrompt(
		"projection", renderDatabaseQueryAuthority(state, false, "PROJECTIONS"),
	), nil
}

func BuildDatabaseQueryProjectionAggregatePrompt(input DatabaseQueryProjectionLeafInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		"Select the one aggregate operation for the next projection, or none when that projection is non-aggregate.",
		"Return exactly one registered value: none, count_rows, count, count_distinct, sum, average, minimum, or maximum. Return no JSON, quotes, label, SQL, explanation, or commentary.",
		renderDatabaseQueryAuthority(input.State, true, "PROJECTIONS"),
	), nil
}

func BuildDatabaseQueryProjectionFieldPrompt(input DatabaseQueryProjectionLeafInput) (string, error) {
	if err := input.validateForField(); err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		fmt.Sprintf("Select the one opaque field ID for the next projection whose aggregate choice is %q.", emptyDatabaseQueryValue(string(input.Aggregate))),
		"Return exactly one projected field ID. Return no JSON, quotes, label, SQL, explanation, or commentary.",
		renderDatabaseQueryAuthority(input.State, true, "PROJECTIONS"),
	), nil
}

func BuildDatabaseQueryProjectionTimeBucketPrompt(input DatabaseQueryProjectionLeafInput) (string, error) {
	if err := input.validateForTimeBucket(); err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		fmt.Sprintf("Select whether field %q is projected directly or as one time bucket.", input.FieldID),
		"Return exactly one registered value: none, day, week, month, quarter, or year. Return no JSON, quotes, label, SQL, explanation, or commentary.",
		renderDatabaseQueryAuthority(input.State, false, "PROJECTIONS"),
	), nil
}

func BuildDatabaseQueryFilterCoveragePrompt(input DatabaseQueryFilterLeafInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	label := "top-level filter"
	if input.ScopeRelationID != "" {
		label = "filter inside the current existence relation"
	}
	return databaseQueryCoveragePrompt(
		label, renderDatabaseQueryFilterAuthority(input, false),
	), nil
}

func BuildDatabaseQueryFilterFieldPrompt(input DatabaseQueryFilterLeafInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		"Select the one opaque field ID constrained by the next filter.",
		"Return exactly one projected field ID. Return no JSON, quotes, label, SQL, explanation, or commentary.",
		renderDatabaseQueryFilterAuthority(input, true),
	), nil
}

func BuildDatabaseQueryFilterOperatorPrompt(input DatabaseQueryFilterLeafInput) (string, error) {
	if err := input.validateField(); err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		fmt.Sprintf("Select the one comparison relation required for filter field %q.", input.FieldID),
		"Return exactly one registered operator shown in the authority. Return no JSON, quotes, label, SQL, explanation, or commentary.",
		renderDatabaseQueryFilterAuthority(input, true),
	), nil
}

func BuildDatabaseQueryFilterValueCoveragePrompt(input DatabaseQueryFilterLeafInput) (string, error) {
	if err := input.validateOperator(); err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		"Answer whether another distinct literal value is required by this bounded set-membership filter.",
		"Return VALUE_REMAINS or NO_UNCOVERED_VALUE exactly. Return no literal, JSON, quotes, label, SQL, explanation, or commentary.",
		renderDatabaseQueryFilterAuthority(input, false),
	), nil
}

func BuildDatabaseQueryFilterValuePrompt(input DatabaseQueryFilterLeafInput) (string, error) {
	if err := input.validateOperator(); err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		fmt.Sprintf("Return the one exact literal value required next for field %q and operator %q.", input.FieldID, input.Operator),
		"Return only the raw literal value on one line. Return no type, second value, JSON, quotes, label, SQL, explanation, or commentary.",
		renderDatabaseQueryFilterAuthority(input, false),
	), nil
}

func BuildDatabaseQueryWindowCoveragePrompt(state DatabaseQueryIntentLeafState) (string, error) {
	if err := state.validateReady(); err != nil {
		return "", err
	}
	return databaseQueryCoveragePrompt(
		"temporal window", renderDatabaseQueryAuthority(state, false, "TEMPORAL WINDOWS"),
	), nil
}

func BuildDatabaseQueryWindowFieldPrompt(input DatabaseQueryWindowLeafInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		"Select the one opaque temporal field ID constrained by the next relative window.",
		"Return exactly one projected temporal field ID. Return no JSON, quotes, label, SQL, explanation, or commentary.",
		renderDatabaseQueryAuthority(input.State, true, "TEMPORAL WINDOWS"),
	), nil
}

func BuildDatabaseQueryWindowUnitPrompt(input DatabaseQueryWindowLeafInput) (string, error) {
	if err := input.validateField(); err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		fmt.Sprintf("Select the one relative time unit for temporal field %q.", input.FieldID),
		"Return exactly one registered value: hour, day, week, month, or year. Return no amount, JSON, quotes, label, SQL, explanation, or commentary.",
		renderDatabaseQueryAuthority(input.State, false, "TEMPORAL WINDOWS"),
	), nil
}

func BuildDatabaseQueryWindowAmountPrompt(input DatabaseQueryWindowLeafInput) (string, error) {
	if err := input.validateUnit(); err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		fmt.Sprintf("Return the one positive integer amount for the %q window over field %q.", input.Unit, input.FieldID),
		"Return exactly one base-10 integer within 1..10000. Return no unit, JSON, quotes, label, SQL, explanation, or commentary.",
		renderDatabaseQueryAuthority(input.State, false, "TEMPORAL WINDOWS"),
	), nil
}

package assemblyline

import "fmt"

func BuildDatabaseQueryExistenceCoveragePrompt(state DatabaseQueryIntentLeafState) (string, error) {
	if err := state.validateReady(); err != nil {
		return "", err
	}
	return databaseQueryCoveragePrompt(
		"existence predicate", renderDatabaseQueryAuthority(state, false, "EXISTENCE PREDICATES"),
	), nil
}

func BuildDatabaseQueryExistenceRelationPrompt(input DatabaseQueryExistenceLeafInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		"Select the one opaque relation ID whose row existence is tested by the next predicate.",
		"Return exactly one projected relation ID. Return no negation, filter, JSON, quotes, label, SQL, explanation, or commentary.",
		renderDatabaseQueryAuthority(input.State, true, "EXISTENCE PREDICATES"),
	), nil
}

func BuildDatabaseQueryExistenceNegatedPrompt(input DatabaseQueryExistenceLeafInput) (string, error) {
	if err := input.validateRelation(); err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		fmt.Sprintf("Select whether rows in relation %q must exist or must not exist.", input.RelationID),
		"Return exactly EXISTS or NOT_EXISTS. Return no filter, JSON, quotes, label, SQL, explanation, or commentary.",
		renderDatabaseQueryAuthority(input.State, false, "EXISTENCE PREDICATES"),
	), nil
}

func BuildDatabaseQueryHavingCoveragePrompt(state DatabaseQueryIntentLeafState) (string, error) {
	if err := state.validateReady(); err != nil {
		return "", err
	}
	return databaseQueryCoveragePrompt(
		"aggregate having predicate", renderDatabaseQueryAuthority(state, false, "HAVING PREDICATES"),
	), nil
}

func BuildDatabaseQueryHavingAggregatePrompt(input DatabaseQueryHavingLeafInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		"Select the one aggregate measured by the next having predicate.",
		"Return exactly one registered value: count_rows, count, count_distinct, sum, or average. Return no field, JSON, quotes, label, SQL, explanation, or commentary.",
		renderDatabaseQueryAuthority(input.State, true, "HAVING PREDICATES"),
	), nil
}

func BuildDatabaseQueryHavingFieldPrompt(input DatabaseQueryHavingLeafInput) (string, error) {
	if err := input.validateAggregate(); err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		fmt.Sprintf("Select the one opaque field ID measured by having aggregate %q.", input.Aggregate),
		"Return exactly one projected field ID. Return no operator, value, JSON, quotes, label, SQL, explanation, or commentary.",
		renderDatabaseQueryAuthority(input.State, true, "HAVING PREDICATES"),
	), nil
}

func BuildDatabaseQueryHavingOperatorPrompt(input DatabaseQueryHavingLeafInput) (string, error) {
	if err := input.validateField(); err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		"Select the one numeric comparison relation for the current having predicate.",
		"Return exactly eq, neq, gt, gte, lt, or lte. Return no value, JSON, quotes, label, SQL, explanation, or commentary.",
		renderDatabaseQueryAuthority(input.State, false, "HAVING PREDICATES"),
	), nil
}

func BuildDatabaseQueryHavingValuePrompt(input DatabaseQueryHavingLeafInput) (string, error) {
	if err := input.validateOperator(); err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		"Return the one exact numeric literal compared by the current having predicate.",
		"Return exactly one base-10 integer or decimal. Return no type, JSON, quotes, label, SQL, explanation, or commentary.",
		renderDatabaseQueryAuthority(input.State, false, "HAVING PREDICATES"),
	), nil
}

func BuildDatabaseQueryOrderCoveragePrompt(state DatabaseQueryIntentLeafState) (string, error) {
	if err := state.validateReady(); err != nil {
		return "", err
	}
	return databaseQueryCoveragePrompt(
		"ordering term", renderDatabaseQueryAuthority(state, false, "ORDER TERMS"),
	), nil
}

func BuildDatabaseQueryOrderProjectionPrompt(input DatabaseQueryOrderLeafInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		"Select the one opaque projection index ordered by the next ordering term.",
		"Return exactly one zero-based projection index shown in the authority. Return no direction, JSON, quotes, label, SQL, explanation, or commentary.",
		renderDatabaseQueryAuthority(input.State, false, "ORDER TERMS"),
	), nil
}

func BuildDatabaseQueryOrderDirectionPrompt(input DatabaseQueryOrderLeafInput) (string, error) {
	if err := input.validateProjection(); err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		fmt.Sprintf("Select the one ordering direction for projection index %d.", *input.Projection),
		"Return exactly asc or desc. Return no index, JSON, quotes, label, SQL, explanation, or commentary.",
		renderDatabaseQueryAuthority(input.State, false, "ORDER TERMS"),
	), nil
}

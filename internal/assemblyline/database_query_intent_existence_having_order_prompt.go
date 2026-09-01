package assemblyline

import "github.com/gryph/omnidex/internal/datasource"

func BuildDatabaseQueryExistenceRelationPrompt(input DatabaseQueryExistenceLeafInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryExistenceAuthority(input)
	if err != nil {
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
	return databaseQueryOpaqueChoicePrompt(
		"Which relation's row existence is tested by the focused accepted purpose?",
		authority,
		choices,
	)
}

func BuildDatabaseQueryExistenceNegatedPrompt(input DatabaseQueryExistenceLeafInput) (string, error) {
	if err := input.validateRelation(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryExistenceAuthority(input)
	if err != nil {
		return "", err
	}
	focused, err := renderDatabaseQueryFocusedRelation(input.State, input.RelationID)
	if err != nil {
		return "", err
	}
	choices, err := databaseQueryExistenceNegatedChoices()
	if err != nil {
		return "", err
	}
	return databaseQueryOpaqueChoicePrompt(
		"Must matching rows in the focused relation exist or not exist?",
		extendDatabaseQueryAuthority(authority, focused),
		choices,
	)
}

func BuildDatabaseQueryHavingAggregatePrompt(input DatabaseQueryHavingLeafInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryHavingAuthority(input, true)
	if err != nil {
		return "", err
	}
	choices, err := databaseQueryHavingAggregateChoices()
	if err != nil {
		return "", err
	}
	return databaseQueryOpaqueChoicePrompt(
		"Which aggregate is measured by the focused accepted having purpose?",
		authority,
		choices,
	)
}

func BuildDatabaseQueryHavingFieldPrompt(input DatabaseQueryHavingLeafInput) (string, error) {
	if err := input.validateAggregate(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryHavingAuthority(input, false)
	if err != nil {
		return "", err
	}
	choices, err := databaseQueryFieldChoices(
		input.State, "", databaseQueryAggregateFieldEligible(input.Aggregate),
	)
	if err != nil {
		return "", err
	}
	aggregate, err := databaseQueryAggregateDescription(input.Aggregate)
	if err != nil {
		return "", err
	}
	return databaseQueryOpaqueChoicePrompt(
		"Which field is measured by the accepted having aggregate?",
		extendDatabaseQueryAuthority(authority, "FOCUSED AGGREGATE:\n"+aggregate),
		choices,
	)
}

func BuildDatabaseQueryHavingOperatorPrompt(input DatabaseQueryHavingLeafInput) (string, error) {
	if err := input.validateField(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryHavingAuthority(input, false)
	if err != nil {
		return "", err
	}
	focused, err := renderDatabaseQueryFocusedHaving(input)
	if err != nil {
		return "", err
	}
	choices, err := databaseQueryHavingOperatorChoices()
	if err != nil {
		return "", err
	}
	return databaseQueryOpaqueChoicePrompt(
		"Which numeric comparison relation does the current having predicate require?",
		extendDatabaseQueryAuthority(authority, focused),
		choices,
	)
}

func BuildDatabaseQueryHavingValuePrompt(input DatabaseQueryHavingLeafInput) (string, error) {
	if err := input.validateOperator(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryHavingAuthority(input, false)
	if err != nil {
		return "", err
	}
	focused, err := renderDatabaseQueryFocusedHaving(input)
	if err != nil {
		return "", err
	}
	operator, err := databaseQueryFilterOperatorDescription(input.Operator)
	if err != nil {
		return "", err
	}
	return databaseQueryPlainTextPrompt(
		"What numeric threshold does the focused having predicate require?",
		extendDatabaseQueryAuthority(
			authority, focused, "ACCEPTED HAVING RELATION:\n"+operator,
		),
	), nil
}

func BuildDatabaseQueryOrderProjectionPrompt(input DatabaseQueryOrderLeafInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryOrderAuthority(input)
	if err != nil {
		return "", err
	}
	choices, err := databaseQueryOrderProjectionChoices(input)
	if err != nil {
		return "", err
	}
	return databaseQueryOpaqueChoicePrompt(
		"Which accepted projection is ordered by the focused accepted ordering purpose?",
		authority,
		choices,
	)
}

func BuildDatabaseQueryOrderDirectionPrompt(input DatabaseQueryOrderLeafInput) (string, error) {
	if err := input.validateProjection(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryOrderAuthority(input)
	if err != nil {
		return "", err
	}
	focused, err := renderDatabaseQueryFocusedProjection(input.State, *input.Projection)
	if err != nil {
		return "", err
	}
	choices, err := databaseQueryOrderDirectionChoices()
	if err != nil {
		return "", err
	}
	return databaseQueryOpaqueChoicePrompt(
		"Which ordering direction does the focused projection require?",
		extendDatabaseQueryAuthority(authority, focused),
		choices,
	)
}

func renderDatabaseQueryExistenceAuthority(input DatabaseQueryExistenceLeafInput) (string, error) {
	accepted, err := renderDatabaseQueryAcceptedQuery(input.State)
	if err != nil {
		return "", err
	}
	existence, err := renderDatabaseQueryAcceptedExistence(input.State)
	if err != nil {
		return "", err
	}
	return renderDatabaseQueryFocusedParameterAuthority(
		input.Purpose, "existence", accepted, existence,
	), nil
}

func renderDatabaseQueryHavingAuthority(
	input DatabaseQueryHavingLeafInput,
	includeSemanticFields bool,
) (string, error) {
	accepted, err := renderDatabaseQueryAcceptedQuery(input.State)
	if err != nil {
		return "", err
	}
	projections, err := renderDatabaseQueryAcceptedProjections(input.State)
	if err != nil {
		return "", err
	}
	having, err := renderDatabaseQueryAcceptedHaving(input.State)
	if err != nil {
		return "", err
	}
	sections := []string{accepted, projections, having}
	if includeSemanticFields {
		sections = append(sections, renderDatabaseQuerySemanticFields(input.State))
	}
	return renderDatabaseQueryFocusedParameterAuthority(
		input.Purpose, "having", sections...,
	), nil
}

func renderDatabaseQueryFocusedHaving(input DatabaseQueryHavingLeafInput) (string, error) {
	if input.Aggregate == datasource.AggregateCountRows {
		return "FOCUSED HAVING MEASURE:\ncount matching rows", nil
	}
	field, err := databaseQueryFieldSemantic(input.State, input.FieldID)
	if err != nil {
		return "", err
	}
	aggregate, err := databaseQueryAggregateDescription(input.Aggregate)
	if err != nil {
		return "", err
	}
	return "FOCUSED HAVING MEASURE:\n" + aggregate + " for " + field, nil
}

func renderDatabaseQueryOrderAuthority(input DatabaseQueryOrderLeafInput) (string, error) {
	accepted, err := renderDatabaseQueryAcceptedQuery(input.State)
	if err != nil {
		return "", err
	}
	projections, err := renderDatabaseQueryAcceptedProjections(input.State)
	if err != nil {
		return "", err
	}
	order, err := renderDatabaseQueryAcceptedOrder(input.State)
	if err != nil {
		return "", err
	}
	return renderDatabaseQueryFocusedParameterAuthority(
		input.Purpose, "order", accepted, projections, order,
	), nil
}

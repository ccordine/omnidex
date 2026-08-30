package assemblyline

import (
	"fmt"

	"github.com/gryph/omnidex/internal/datasource"
)

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
	authority = extendDatabaseQueryAuthority(
		authority, renderDatabaseQueryRelationCandidates(input.State, excluded),
	)
	return databaseQueryLeafPrompt(
		"Select the one opaque relation ID whose row existence is tested by the focused accepted existence purpose.",
		"Return exactly one projected relation ID as a raw line.",
		authority,
	), nil
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
	return databaseQueryLeafPrompt(
		"Select whether rows in the focused relation must exist or must not exist.",
		"Return exactly one raw registered value: EXISTS or NOT_EXISTS.",
		extendDatabaseQueryAuthority(authority, focused),
	), nil
}

func BuildDatabaseQueryHavingAggregatePrompt(input DatabaseQueryHavingLeafInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryHavingAuthority(input, true)
	if err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		"Select the one aggregate measured by the focused accepted having purpose.",
		"Return exactly one raw registered value: count_rows, count, count_distinct, sum, or average.",
		authority,
	), nil
}

func BuildDatabaseQueryHavingFieldPrompt(input DatabaseQueryHavingLeafInput) (string, error) {
	if err := input.validateAggregate(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryHavingAuthority(input, false)
	if err != nil {
		return "", err
	}
	authority = extendDatabaseQueryAuthority(
		authority,
		"ACCEPTED HAVING AGGREGATE:\n"+string(input.Aggregate),
		renderDatabaseQueryFieldCandidates(
			input.State, "", databaseQueryAggregateFieldEligible(input.Aggregate),
		),
	)
	return databaseQueryLeafPrompt(
		"Select the one opaque field ID measured by the accepted having aggregate.",
		"Return exactly one projected field ID as a raw line.",
		authority,
	), nil
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
	return databaseQueryLeafPrompt(
		"Select the one numeric comparison relation for the current having predicate.",
		"Return exactly one raw registered value: eq, neq, gt, gte, lt, or lte.",
		extendDatabaseQueryAuthority(authority, focused),
	), nil
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
	return databaseQueryLeafPrompt(
		"Return the one exact numeric literal compared by the current having predicate.",
		"Return exactly one raw base-10 integer or decimal.",
		extendDatabaseQueryAuthority(
			authority, focused, "ACCEPTED HAVING OPERATOR:\n"+string(input.Operator),
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
	candidates, err := renderDatabaseQueryProjectionCandidates(input.State)
	if err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		"Select the one opaque projection index ordered by the focused accepted ordering purpose.",
		"Return exactly one raw zero-based projection index shown in the authority.",
		extendDatabaseQueryAuthority(authority, candidates),
	), nil
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
	return databaseQueryLeafPrompt(
		"Select the one ordering direction for the focused projection.",
		"Return exactly one raw registered value: asc or desc.",
		extendDatabaseQueryAuthority(authority, focused),
	), nil
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
		return "FOCUSED HAVING MEASURE:\naggregate=count_rows field=rows", nil
	}
	field, err := databaseQueryFieldSemantic(input.State, input.FieldID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"FOCUSED HAVING MEASURE:\naggregate=%s field=%s", input.Aggregate, field,
	), nil
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

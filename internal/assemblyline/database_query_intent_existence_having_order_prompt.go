package assemblyline

import (
	"fmt"

	"github.com/gryph/omnidex/internal/datasource"
)

func BuildDatabaseQueryExistenceCoveragePrompt(state DatabaseQueryIntentLeafState) (string, error) {
	if err := state.validateReady(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryExistenceAuthority(state)
	if err != nil {
		return "", err
	}
	return databaseQueryCoveragePrompt(
		"existence predicate", authority,
	), nil
}

func BuildDatabaseQueryExistenceRelationPrompt(input DatabaseQueryExistenceLeafInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryExistenceAuthority(input.State)
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
		"Select the one opaque relation ID whose row existence is tested by the next predicate.",
		"Return exactly one projected relation ID. Return no negation, filter, JSON, quotes, label, SQL, explanation, or commentary.",
		authority,
	), nil
}

func BuildDatabaseQueryExistenceNegatedPrompt(input DatabaseQueryExistenceLeafInput) (string, error) {
	if err := input.validateRelation(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryExistenceAuthority(input.State)
	if err != nil {
		return "", err
	}
	focused, err := renderDatabaseQueryFocusedRelation(input.State, input.RelationID)
	if err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		"Select whether rows in the focused relation must exist or must not exist.",
		"Return exactly EXISTS or NOT_EXISTS. Return no filter, JSON, quotes, label, SQL, explanation, or commentary.",
		extendDatabaseQueryAuthority(authority, focused),
	), nil
}

func BuildDatabaseQueryHavingCoveragePrompt(state DatabaseQueryIntentLeafState) (string, error) {
	if err := state.validateReady(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryHavingAuthority(state, false)
	if err != nil {
		return "", err
	}
	return databaseQueryCoveragePrompt(
		"aggregate having predicate", authority,
	), nil
}

func BuildDatabaseQueryHavingAggregatePrompt(input DatabaseQueryHavingLeafInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryHavingAuthority(input.State, true)
	if err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		"Select the one aggregate measured by the next having predicate.",
		"Return exactly one registered value: count_rows, count, count_distinct, sum, or average. Return no field, JSON, quotes, label, SQL, explanation, or commentary.",
		authority,
	), nil
}

func BuildDatabaseQueryHavingFieldPrompt(input DatabaseQueryHavingLeafInput) (string, error) {
	if err := input.validateAggregate(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryHavingAuthority(input.State, false)
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
		"Return exactly one projected field ID. Return no operator, value, JSON, quotes, label, SQL, explanation, or commentary.",
		authority,
	), nil
}

func BuildDatabaseQueryHavingOperatorPrompt(input DatabaseQueryHavingLeafInput) (string, error) {
	if err := input.validateField(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryHavingAuthority(input.State, false)
	if err != nil {
		return "", err
	}
	focused, err := renderDatabaseQueryFocusedHaving(input)
	if err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		"Select the one numeric comparison relation for the current having predicate.",
		"Return exactly eq, neq, gt, gte, lt, or lte. Return no value, JSON, quotes, label, SQL, explanation, or commentary.",
		extendDatabaseQueryAuthority(authority, focused),
	), nil
}

func BuildDatabaseQueryHavingValuePrompt(input DatabaseQueryHavingLeafInput) (string, error) {
	if err := input.validateOperator(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryHavingAuthority(input.State, false)
	if err != nil {
		return "", err
	}
	focused, err := renderDatabaseQueryFocusedHaving(input)
	if err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		"Return the one exact numeric literal compared by the current having predicate.",
		"Return exactly one base-10 integer or decimal. Return no type, JSON, quotes, label, SQL, explanation, or commentary.",
		extendDatabaseQueryAuthority(
			authority, focused, "ACCEPTED HAVING OPERATOR:\n"+string(input.Operator),
		),
	), nil
}

func BuildDatabaseQueryOrderCoveragePrompt(state DatabaseQueryIntentLeafState) (string, error) {
	if err := state.validateReady(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryOrderAuthority(state)
	if err != nil {
		return "", err
	}
	return databaseQueryCoveragePrompt(
		"ordering term", authority,
	), nil
}

func BuildDatabaseQueryOrderProjectionPrompt(input DatabaseQueryOrderLeafInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryOrderAuthority(input.State)
	if err != nil {
		return "", err
	}
	candidates, err := renderDatabaseQueryProjectionCandidates(input.State)
	if err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		"Select the one opaque projection index ordered by the next ordering term.",
		"Return exactly one zero-based projection index shown in the authority. Return no direction, JSON, quotes, label, SQL, explanation, or commentary.",
		extendDatabaseQueryAuthority(authority, candidates),
	), nil
}

func BuildDatabaseQueryOrderDirectionPrompt(input DatabaseQueryOrderLeafInput) (string, error) {
	if err := input.validateProjection(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryOrderAuthority(input.State)
	if err != nil {
		return "", err
	}
	focused, err := renderDatabaseQueryFocusedProjection(input.State, *input.Projection)
	if err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		"Select the one ordering direction for the focused projection.",
		"Return exactly asc or desc. Return no index, JSON, quotes, label, SQL, explanation, or commentary.",
		extendDatabaseQueryAuthority(authority, focused),
	), nil
}

func renderDatabaseQueryExistenceAuthority(state DatabaseQueryIntentLeafState) (string, error) {
	accepted, err := renderDatabaseQueryAcceptedQuery(state)
	if err != nil {
		return "", err
	}
	existence, err := renderDatabaseQueryAcceptedExistence(state)
	if err != nil {
		return "", err
	}
	return renderDatabaseQueryAuthority(state, accepted, existence), nil
}

func renderDatabaseQueryHavingAuthority(
	state DatabaseQueryIntentLeafState,
	includeSemanticFields bool,
) (string, error) {
	accepted, err := renderDatabaseQueryAcceptedQuery(state)
	if err != nil {
		return "", err
	}
	projections, err := renderDatabaseQueryAcceptedProjections(state)
	if err != nil {
		return "", err
	}
	having, err := renderDatabaseQueryAcceptedHaving(state)
	if err != nil {
		return "", err
	}
	sections := []string{accepted, projections, having}
	if includeSemanticFields {
		sections = append(sections, renderDatabaseQuerySemanticFields(state))
	}
	return renderDatabaseQueryAuthority(state, sections...), nil
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

func renderDatabaseQueryOrderAuthority(state DatabaseQueryIntentLeafState) (string, error) {
	accepted, err := renderDatabaseQueryAcceptedQuery(state)
	if err != nil {
		return "", err
	}
	projections, err := renderDatabaseQueryAcceptedProjections(state)
	if err != nil {
		return "", err
	}
	order, err := renderDatabaseQueryAcceptedOrder(state)
	if err != nil {
		return "", err
	}
	return renderDatabaseQueryAuthority(state, accepted, projections, order), nil
}

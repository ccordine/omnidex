package assemblyline

import (
	"fmt"
	"strings"
)

func BuildDatabaseQueryFromRelationPrompt(state DatabaseQueryIntentLeafState) (string, error) {
	if err := state.validate(); err != nil {
		return "", err
	}
	authority := renderDatabaseQueryAuthority(
		state, renderDatabaseQueryRelationCandidates(state, nil),
	)
	return databaseQueryLeafPrompt(
		"Select the one opaque relation ID that should anchor the relational query for the exact evidence need.",
		"Return exactly one projected relation ID as a raw line.",
		authority,
	), nil
}

func BuildDatabaseQueryShapePrompt(state DatabaseQueryIntentLeafState) (string, error) {
	if err := state.validate(); err != nil {
		return "", err
	}
	if state.FromRelationID == "" {
		return "", fmt.Errorf("database query shape requires an accepted from relation")
	}
	accepted, err := renderDatabaseQueryAcceptedQuery(state)
	if err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		"Select the one result shape that directly answers the exact evidence need.",
		"Return exactly one raw registered value: records, scalar, ranking, distribution, comparison, trend, or existence.",
		renderDatabaseQueryAuthority(state, accepted),
	), nil
}

func BuildDatabaseQueryProjectionAggregatePrompt(input DatabaseQueryProjectionLeafInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryProjectionAuthority(input, true)
	if err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		"Select the one aggregate operation that implements the focused projection purpose, or none when that projection is non-aggregate.",
		"Return exactly one raw registered value: none, count_rows, count, count_distinct, sum, average, minimum, or maximum.",
		authority,
	), nil
}

func BuildDatabaseQueryProjectionFieldPrompt(input DatabaseQueryProjectionLeafInput) (string, error) {
	if err := input.validateForField(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryProjectionAuthority(input, false)
	if err != nil {
		return "", err
	}
	mode := "direct field"
	if input.Aggregate != "" {
		mode = string(input.Aggregate) + " aggregate"
	}
	authority = extendDatabaseQueryAuthority(
		authority,
		renderDatabaseQueryFieldCandidates(
			input.State, "", databaseQueryAggregateFieldEligible(input.Aggregate),
		),
	)
	return databaseQueryLeafPrompt(
		fmt.Sprintf("Select the one opaque field ID that implements the focused %s projection purpose.", mode),
		"Return exactly one projected field ID as a raw line.",
		authority,
	), nil
}

func BuildDatabaseQueryProjectionTimeBucketPrompt(input DatabaseQueryProjectionLeafInput) (string, error) {
	if err := input.validateForTimeBucket(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryProjectionAuthority(input, false)
	if err != nil {
		return "", err
	}
	focused, err := renderDatabaseQueryFocusedField(input.State, input.FieldID)
	if err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		"Select whether the focused field is projected directly or as one time bucket.",
		"Return exactly one raw registered value: none, day, week, month, quarter, or year.",
		extendDatabaseQueryAuthority(authority, focused),
	), nil
}

func BuildDatabaseQueryFilterFieldPrompt(input DatabaseQueryFilterLeafInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryFilterAuthority(input, false, false, false)
	if err != nil {
		return "", err
	}
	authority = extendDatabaseQueryAuthority(
		authority,
		renderDatabaseQueryFieldCandidates(input.State, input.ScopeRelationID, nil),
	)
	return databaseQueryLeafPrompt(
		"Select the one opaque field ID constrained by the focused accepted filter purpose.",
		"Return exactly one projected field ID as a raw line.",
		authority,
	), nil
}

func BuildDatabaseQueryFilterOperatorPrompt(input DatabaseQueryFilterLeafInput) (string, error) {
	if err := input.validateField(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryFilterAuthority(input, true, false, true)
	if err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		"Select the one comparison relation required by the focused accepted filter purpose.",
		"Return exactly one raw registered operator shown in the authority.",
		authority,
	), nil
}

func BuildDatabaseQueryFilterValuePrompt(input DatabaseQueryFilterLeafInput) (string, error) {
	if err := input.validateOperator(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryFilterAuthority(input, true, true, false)
	if err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		"Return the one exact literal value that implements the focused accepted purpose for the focused field and operator.",
		"Return exactly one raw literal value on one line.",
		authority,
	), nil
}

func BuildDatabaseQueryWindowFieldPrompt(input DatabaseQueryWindowLeafInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryWindowAuthority(input)
	if err != nil {
		return "", err
	}
	authority = extendDatabaseQueryAuthority(
		authority,
		renderDatabaseQueryFieldCandidates(input.State, "", databaseQueryTemporalFieldEligible),
	)
	return databaseQueryLeafPrompt(
		"Select the one opaque temporal field ID constrained by the focused accepted temporal-window purpose.",
		"Return exactly one projected temporal field ID as a raw line.",
		authority,
	), nil
}

func BuildDatabaseQueryWindowUnitPrompt(input DatabaseQueryWindowLeafInput) (string, error) {
	if err := input.validateField(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryWindowAuthority(input)
	if err != nil {
		return "", err
	}
	focused, err := renderDatabaseQueryFocusedField(input.State, input.FieldID)
	if err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		"Select the one relative time unit for the focused temporal field.",
		"Return exactly one raw registered value: hour, day, week, month, or year.",
		extendDatabaseQueryAuthority(authority, focused),
	), nil
}

func BuildDatabaseQueryWindowAmountPrompt(input DatabaseQueryWindowLeafInput) (string, error) {
	if err := input.validateUnit(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryWindowAuthority(input)
	if err != nil {
		return "", err
	}
	focused, err := renderDatabaseQueryFocusedField(input.State, input.FieldID)
	if err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		"Return the one positive integer amount for the accepted window unit over the focused field.",
		"Return exactly one raw base-10 integer within 1..10000.",
		extendDatabaseQueryAuthority(
			authority, focused, "ACCEPTED WINDOW UNIT:\n"+string(input.Unit),
		),
	), nil
}

func renderDatabaseQueryProjectionAuthority(
	input DatabaseQueryProjectionLeafInput,
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
	sections := []string{accepted, projections}
	if includeSemanticFields {
		sections = append(sections, renderDatabaseQuerySemanticFields(input.State))
	}
	return renderDatabaseQueryFocusedParameterAuthority(
		input.Purpose, "projection", sections...,
	), nil
}

func renderDatabaseQueryFilterAuthority(
	input DatabaseQueryFilterLeafInput,
	includeFocusedField bool,
	includeAcceptedValues bool,
	includeOperators bool,
) (string, error) {
	accepted, err := renderDatabaseQueryAcceptedQuery(input.State)
	if err != nil {
		return "", err
	}
	scope, err := renderDatabaseQueryFilterScope(input)
	if err != nil {
		return "", err
	}
	filters, err := renderDatabaseQueryAcceptedFilters(input.State, input.AcceptedFilters)
	if err != nil {
		return "", err
	}
	sections := []string{accepted, scope, filters}
	if input.ParentPurpose != "" {
		sections = append(sections, "ACCEPTED PARENT QUERY PURPOSE:\n"+input.ParentPurpose)
	}
	if includeFocusedField {
		focused, err := renderDatabaseQueryFocusedField(input.State, input.FieldID)
		if err != nil {
			return "", err
		}
		sections = append(sections, focused, renderDatabaseQueryFilterOperator(input))
	}
	if includeAcceptedValues {
		sections = append(sections, renderDatabaseQueryAcceptedValues(input))
	}
	if includeOperators {
		sections = append(sections, renderDatabaseQueryAllowedFilterOperators(input))
	}
	return renderDatabaseQueryFocusedParameterAuthority(
		input.Purpose, "filter", sections...,
	), nil
}

func renderDatabaseQueryFocusedPurpose(purpose, collection string) string {
	return "FOCUSED ACCEPTED " + strings.ToUpper(collection) + " PURPOSE:\n" + purpose
}

func renderDatabaseQueryWindowAuthority(input DatabaseQueryWindowLeafInput) (string, error) {
	accepted, err := renderDatabaseQueryAcceptedQuery(input.State)
	if err != nil {
		return "", err
	}
	windows, err := renderDatabaseQueryAcceptedWindows(input.State)
	if err != nil {
		return "", err
	}
	return renderDatabaseQueryFocusedParameterAuthority(
		input.Purpose, "temporal-window", accepted, windows,
	), nil
}

package assemblyline

import (
	"fmt"
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
		"Return exactly one projected relation ID. Return no JSON, quotes, label, SQL, explanation, or commentary.",
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
		"Return exactly one registered value: records, scalar, ranking, distribution, comparison, trend, or existence. Return no JSON, quotes, label, SQL, explanation, or commentary.",
		renderDatabaseQueryAuthority(state, accepted),
	), nil
}

func BuildDatabaseQueryProjectionCoveragePrompt(state DatabaseQueryIntentLeafState) (string, error) {
	if err := state.validateReady(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryProjectionAuthority(state, false)
	if err != nil {
		return "", err
	}
	return databaseQueryCoveragePrompt(
		"projection", authority,
	), nil
}

func BuildDatabaseQueryProjectionAggregatePrompt(input DatabaseQueryProjectionLeafInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryProjectionAuthority(input.State, true)
	if err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		"Select the one aggregate operation for the next projection, or none when that projection is non-aggregate.",
		"Return exactly one registered value: none, count_rows, count, count_distinct, sum, average, minimum, or maximum. Return no JSON, quotes, label, SQL, explanation, or commentary.",
		authority,
	), nil
}

func BuildDatabaseQueryProjectionFieldPrompt(input DatabaseQueryProjectionLeafInput) (string, error) {
	if err := input.validateForField(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryProjectionAuthority(input.State, false)
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
		fmt.Sprintf("Select the one opaque field ID for the next %s projection.", mode),
		"Return exactly one projected field ID. Return no JSON, quotes, label, SQL, explanation, or commentary.",
		authority,
	), nil
}

func BuildDatabaseQueryProjectionTimeBucketPrompt(input DatabaseQueryProjectionLeafInput) (string, error) {
	if err := input.validateForTimeBucket(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryProjectionAuthority(input.State, false)
	if err != nil {
		return "", err
	}
	focused, err := renderDatabaseQueryFocusedField(input.State, input.FieldID)
	if err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		"Select whether the focused field is projected directly or as one time bucket.",
		"Return exactly one registered value: none, day, week, month, quarter, or year. Return no JSON, quotes, label, SQL, explanation, or commentary.",
		extendDatabaseQueryAuthority(authority, focused),
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
	authority, err := renderDatabaseQueryFilterAuthority(input, false, false, false)
	if err != nil {
		return "", err
	}
	return databaseQueryCoveragePrompt(
		label, authority,
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
		"Select the one opaque field ID constrained by the next filter.",
		"Return exactly one projected field ID. Return no JSON, quotes, label, SQL, explanation, or commentary.",
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
		"Select the one comparison relation required for the focused filter field.",
		"Return exactly one registered operator shown in the authority. Return no JSON, quotes, label, SQL, explanation, or commentary.",
		authority,
	), nil
}

func BuildDatabaseQueryFilterValueCoveragePrompt(input DatabaseQueryFilterLeafInput) (string, error) {
	if err := input.validateOperator(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryFilterAuthority(input, true, true, false)
	if err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		"Answer whether another distinct literal value is required by this bounded set-membership filter.",
		"Return VALUE_REMAINS or NO_UNCOVERED_VALUE exactly. Return no literal, JSON, quotes, label, SQL, explanation, or commentary.",
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
		"Return the one exact literal value required next for the focused field and accepted operator.",
		"Return only the raw literal value on one line. Return no type, second value, JSON, quotes, label, SQL, explanation, or commentary.",
		authority,
	), nil
}

func BuildDatabaseQueryWindowCoveragePrompt(state DatabaseQueryIntentLeafState) (string, error) {
	if err := state.validateReady(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryWindowAuthority(state)
	if err != nil {
		return "", err
	}
	return databaseQueryCoveragePrompt(
		"temporal window", authority,
	), nil
}

func BuildDatabaseQueryWindowFieldPrompt(input DatabaseQueryWindowLeafInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryWindowAuthority(input.State)
	if err != nil {
		return "", err
	}
	authority = extendDatabaseQueryAuthority(
		authority,
		renderDatabaseQueryFieldCandidates(input.State, "", databaseQueryTemporalFieldEligible),
	)
	return databaseQueryLeafPrompt(
		"Select the one opaque temporal field ID constrained by the next relative window.",
		"Return exactly one projected temporal field ID. Return no JSON, quotes, label, SQL, explanation, or commentary.",
		authority,
	), nil
}

func BuildDatabaseQueryWindowUnitPrompt(input DatabaseQueryWindowLeafInput) (string, error) {
	if err := input.validateField(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryWindowAuthority(input.State)
	if err != nil {
		return "", err
	}
	focused, err := renderDatabaseQueryFocusedField(input.State, input.FieldID)
	if err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		"Select the one relative time unit for the focused temporal field.",
		"Return exactly one registered value: hour, day, week, month, or year. Return no amount, JSON, quotes, label, SQL, explanation, or commentary.",
		extendDatabaseQueryAuthority(authority, focused),
	), nil
}

func BuildDatabaseQueryWindowAmountPrompt(input DatabaseQueryWindowLeafInput) (string, error) {
	if err := input.validateUnit(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryWindowAuthority(input.State)
	if err != nil {
		return "", err
	}
	focused, err := renderDatabaseQueryFocusedField(input.State, input.FieldID)
	if err != nil {
		return "", err
	}
	return databaseQueryLeafPrompt(
		"Return the one positive integer amount for the accepted window unit over the focused field.",
		"Return exactly one base-10 integer within 1..10000. Return no unit, JSON, quotes, label, SQL, explanation, or commentary.",
		extendDatabaseQueryAuthority(
			authority, focused, "ACCEPTED WINDOW UNIT:\n"+string(input.Unit),
		),
	), nil
}

func renderDatabaseQueryProjectionAuthority(
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
	sections := []string{accepted, projections}
	if includeSemanticFields {
		sections = append(sections, renderDatabaseQuerySemanticFields(state))
	}
	return renderDatabaseQueryAuthority(state, sections...), nil
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
	return renderDatabaseQueryAuthority(input.State, sections...), nil
}

func renderDatabaseQueryWindowAuthority(state DatabaseQueryIntentLeafState) (string, error) {
	accepted, err := renderDatabaseQueryAcceptedQuery(state)
	if err != nil {
		return "", err
	}
	windows, err := renderDatabaseQueryAcceptedWindows(state)
	if err != nil {
		return "", err
	}
	return renderDatabaseQueryAuthority(state, accepted, windows), nil
}

package assemblyline

import (
	"fmt"
	"strings"
)

func BuildDatabaseQueryFromRelationPrompt(state DatabaseQueryIntentLeafState) (string, error) {
	if err := state.validate(); err != nil {
		return "", err
	}
	choices, err := databaseQueryFromRelationChoices(state)
	if err != nil {
		return "", err
	}
	return databaseQueryOpaqueChoicePrompt(
		"Which relation should anchor the query for the exact evidence need?",
		renderDatabaseQueryAuthority(state),
		choices,
	)
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
	choices, err := databaseQueryShapeChoices()
	if err != nil {
		return "", err
	}
	return databaseQueryOpaqueChoicePrompt(
		"Which result shape directly answers the exact evidence need?",
		renderDatabaseQueryAuthority(state, accepted),
		choices,
	)
}

func BuildDatabaseQueryProjectionAggregatePrompt(input DatabaseQueryProjectionLeafInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryProjectionAuthority(input, true)
	if err != nil {
		return "", err
	}
	choices, err := databaseQueryProjectionAggregateChoices(input)
	if err != nil {
		return "", err
	}
	return databaseQueryOpaqueChoicePrompt(
		"Which operation implements the focused projection purpose?",
		authority,
		choices,
	)
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
		mode, err = databaseQueryAggregateDescription(input.Aggregate)
		if err != nil {
			return "", err
		}
		authority = extendDatabaseQueryAuthority(authority, "FOCUSED OPERATION:\n"+mode)
	}
	choices, err := databaseQueryFieldChoices(
		input.State, "", databaseQueryAggregateFieldEligible(input.Aggregate),
	)
	if err != nil {
		return "", err
	}
	return databaseQueryOpaqueChoicePrompt(
		fmt.Sprintf("Which field implements the focused %s projection purpose?", mode),
		authority,
		choices,
	)
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
	choices, err := databaseQueryProjectionTimeBucketChoices()
	if err != nil {
		return "", err
	}
	return databaseQueryOpaqueChoicePrompt(
		"Should the focused temporal field be used directly or grouped into a calendar bucket?",
		extendDatabaseQueryAuthority(authority, focused),
		choices,
	)
}

func BuildDatabaseQueryFilterFieldPrompt(input DatabaseQueryFilterLeafInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryFilterAuthority(input, false, false)
	if err != nil {
		return "", err
	}
	choices, err := databaseQueryFieldChoices(input.State, input.ScopeRelationID, nil)
	if err != nil {
		return "", err
	}
	return databaseQueryOpaqueChoicePrompt(
		"Which field is constrained by the focused accepted filter purpose?",
		authority,
		choices,
	)
}

func BuildDatabaseQueryFilterOperatorPrompt(input DatabaseQueryFilterLeafInput) (string, error) {
	if err := input.validateField(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryFilterAuthority(input, true, false)
	if err != nil {
		return "", err
	}
	choices, err := databaseQueryFilterOperatorChoices(input)
	if err != nil {
		return "", err
	}
	return databaseQueryOpaqueChoicePrompt(
		"Which comparison relation is required by the focused accepted filter purpose?",
		authority,
		choices,
	)
}

func BuildDatabaseQueryFilterValuePrompt(input DatabaseQueryFilterLeafInput) (string, error) {
	if err := input.validateOperator(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryFilterAuthority(input, true, true)
	if err != nil {
		return "", err
	}
	choices, closed, err := databaseQueryFilterValueChoices(input)
	if err != nil {
		return "", err
	}
	if closed {
		return databaseQueryOpaqueChoicePrompt(
			"Which available value implements the focused accepted filter purpose?",
			authority,
			choices,
		)
	}
	return databaseQueryPlainTextPrompt(
		"What literal value implements the focused accepted purpose for the focused field and comparison relation?",
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
	choices, err := databaseQueryFieldChoices(input.State, "", databaseQueryTemporalFieldEligible)
	if err != nil {
		return "", err
	}
	return databaseQueryOpaqueChoicePrompt(
		"Which temporal field is constrained by the focused accepted temporal-window purpose?",
		authority,
		choices,
	)
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
	choices, err := databaseQueryWindowUnitChoices(input)
	if err != nil {
		return "", err
	}
	return databaseQueryOpaqueChoicePrompt(
		"Which relative time unit does the focused temporal-window purpose require?",
		extendDatabaseQueryAuthority(authority, focused),
		choices,
	)
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
	unit, err := databaseQueryWindowUnitDescription(input.Unit)
	if err != nil {
		return "", err
	}
	return databaseQueryPlainTextPrompt(
		"How many accepted time units does the focused temporal-window purpose require?",
		extendDatabaseQueryAuthority(
			authority, focused, "ACCEPTED WINDOW UNIT:\n"+unit,
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
		operator, err := renderDatabaseQueryFilterOperator(input)
		if err != nil {
			return "", err
		}
		sections = append(sections, focused, operator)
	}
	if includeAcceptedValues {
		sections = append(sections, renderDatabaseQueryAcceptedValues(input))
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

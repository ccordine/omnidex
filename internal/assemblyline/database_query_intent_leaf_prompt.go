package assemblyline

import "fmt"

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
	authority, err := renderDatabaseQuerySelectionAuthority(input.State, input.Purpose, "projection", true)
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
	authority, err := renderDatabaseQuerySelectionAuthority(input.State, input.Purpose, "projection", false)
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
	authority := renderDatabaseQueryFocusedPurpose(input.Purpose, "projection")
	focused, err := renderDatabaseQueryFocusedField(input.State, input.FieldID, false)
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
	authority, err := renderDatabaseQueryFilterFieldAuthority(input)
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
	authority, err := renderDatabaseQueryFilterParameterAuthority(input)
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
	authority, err := renderDatabaseQueryFilterParameterAuthority(input)
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
	authority, err := renderDatabaseQuerySelectionAuthority(input.State, input.Purpose, "temporal-window", false)
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
	authority := renderDatabaseQueryFocusedPurpose(input.Purpose, "temporal-window")
	focused, err := renderDatabaseQueryFocusedField(input.State, input.FieldID, false)
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
	authority := renderDatabaseQueryFocusedPurpose(input.Purpose, "temporal-window")
	focused, err := renderDatabaseQueryFocusedField(input.State, input.FieldID, false)
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

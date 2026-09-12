package assemblyline

import "strings"

func renderDatabaseQuerySelectionAuthority(
	state DatabaseQueryIntentLeafState,
	purpose, collection string,
	includeSemanticFields bool,
) (string, error) {
	anchor, err := renderDatabaseQueryFocusedRelation(state, state.FromRelationID)
	if err != nil {
		return "", err
	}
	sections := []string{anchor}
	if includeSemanticFields {
		sections = append(sections, renderDatabaseQuerySemanticFields(state))
	}
	return renderDatabaseQueryFocusedParameterAuthority(purpose, collection, sections...), nil
}

func renderDatabaseQueryFilterFieldAuthority(input DatabaseQueryFilterLeafInput) (string, error) {
	relationID := input.ScopeRelationID
	if relationID == "" {
		// Top-level predicates use the query anchor; existence predicates have
		// their own explicit relation scope and already-filtered field choices.
		relationID = input.State.FromRelationID
	}
	scope, err := renderDatabaseQueryFocusedRelation(input.State, relationID)
	if err != nil {
		return "", err
	}
	sections := []string{scope}
	if input.ParentPurpose != "" {
		sections = append(sections, "ACCEPTED PARENT QUERY PURPOSE:\n"+input.ParentPurpose)
	}
	return renderDatabaseQueryFocusedParameterAuthority(input.Purpose, "filter", sections...), nil
}

// Comparison and value calls consume one selected field and one local purpose.
// Accepted clauses and values remain in code for validation and choice exclusion.
func renderDatabaseQueryFilterParameterAuthority(input DatabaseQueryFilterLeafInput) (string, error) {
	focused, err := renderDatabaseQueryFocusedField(input.State, input.FieldID, false)
	if err != nil {
		return "", err
	}
	operator, err := renderDatabaseQueryFilterOperator(input)
	if err != nil {
		return "", err
	}
	sections := []string{focused, operator}
	if input.ParentPurpose != "" {
		sections = append(sections, "ACCEPTED PARENT QUERY PURPOSE:\n"+input.ParentPurpose)
	}
	return renderDatabaseQueryFocusedParameterAuthority(input.Purpose, "filter", sections...), nil
}

func renderDatabaseQueryFocusedPurpose(purpose, collection string) string {
	return "FOCUSED ACCEPTED " + strings.ToUpper(collection) + " PURPOSE:\n" + purpose
}

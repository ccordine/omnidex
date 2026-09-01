package assemblyline

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/datasource"
)

func databaseQueryPlainTextPrompt(question, authority string) string {
	return strings.Join([]string{
		strings.TrimSpace(authority),
		strings.TrimSpace(question),
	}, "\n\n")
}

func databaseQueryOpaqueChoicePrompt(
	question string,
	authority string,
	choices []OpaqueModelChoice,
) (string, error) {
	return RenderOpaqueModelChoiceQuestion(
		question,
		[]string{strings.TrimSpace(authority)},
		choices,
	)
}

func renderDatabaseQueryAuthority(state DatabaseQueryIntentLeafState, sections ...string) string {
	var rendered strings.Builder
	fmt.Fprintf(&rendered, "EXACT EVIDENCE NEED:\n%s\n", state.Authority.ExactNeed)
	for index, capsule := range state.Authority.Context.Capsules {
		fmt.Fprintf(&rendered, "CONTEXT CAPSULE %d:\n%s\n", index+1, capsule.Content)
	}
	for _, section := range sections {
		section = strings.TrimSpace(section)
		if section != "" {
			rendered.WriteString(section)
			rendered.WriteByte('\n')
		}
	}
	return strings.TrimSuffix(rendered.String(), "\n")
}

func renderDatabaseQueryFocusedParameterAuthority(
	purpose string,
	collection string,
	sections ...string,
) string {
	parts := []string{renderDatabaseQueryFocusedPurpose(purpose, collection)}
	for _, section := range sections {
		if section = strings.TrimSpace(section); section != "" {
			parts = append(parts, section)
		}
	}
	return strings.Join(parts, "\n")
}

func extendDatabaseQueryAuthority(authority string, sections ...string) string {
	parts := []string{strings.TrimSpace(authority)}
	for _, section := range sections {
		if section = strings.TrimSpace(section); section != "" {
			parts = append(parts, section)
		}
	}
	return strings.Join(parts, "\n")
}

func renderDatabaseQueryAcceptedQuery(state DatabaseQueryIntentLeafState) (string, error) {
	var rendered strings.Builder
	if state.FromRelationID != "" {
		relation, ok := databaseQueryProjectedRelation(state, state.FromRelationID)
		if !ok {
			return "", fmt.Errorf("database query accepted relation %q was not projected", state.FromRelationID)
		}
		fmt.Fprintf(&rendered, "ACCEPTED FROM RELATION:\n%s.%s\n", relation.SchemaName, relation.Name)
	}
	if state.Shape != "" {
		description, err := databaseQueryShapeDescription(state.Shape)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&rendered, "ACCEPTED RESULT SHAPE:\n%s\n", description)
	}
	return strings.TrimSpace(rendered.String()), nil
}

func renderDatabaseQuerySemanticFields(state DatabaseQueryIntentLeafState) string {
	var rendered strings.Builder
	rendered.WriteString("AVAILABLE FIELDS:\n")
	for _, relation := range state.Authority.SchemaProjection.Relations {
		for _, column := range relation.Columns {
			renderDatabaseQueryField(&rendered, "FIELD", relation, column)
		}
	}
	return strings.TrimSpace(rendered.String())
}

func renderDatabaseQueryFocusedRelation(
	state DatabaseQueryIntentLeafState,
	relationID string,
) (string, error) {
	relation, ok := databaseQueryProjectedRelation(state, relationID)
	if !ok {
		return "", fmt.Errorf("database query focused relation %q was not projected", relationID)
	}
	return fmt.Sprintf("FOCUSED RELATION:\n%s.%s", relation.SchemaName, relation.Name), nil
}

func renderDatabaseQueryFocusedField(
	state DatabaseQueryIntentLeafState,
	fieldID string,
) (string, error) {
	column, relationID, ok := databaseQueryColumn(state, fieldID)
	if !ok {
		return "", fmt.Errorf("database query focused field %q was not projected", fieldID)
	}
	relation, ok := databaseQueryProjectedRelation(state, relationID)
	if !ok {
		return "", fmt.Errorf("database query focused field relation %q was not projected", relationID)
	}
	var rendered strings.Builder
	renderDatabaseQueryField(&rendered, "FOCUSED FIELD", relation, column)
	return strings.TrimSpace(rendered.String()), nil
}

func renderDatabaseQueryField(
	rendered *strings.Builder,
	prefix string,
	relation datasource.IntentRelationProjection,
	column datasource.IntentColumnProjection,
) {
	fmt.Fprintf(
		rendered, "%s: %s.%s.%s; type: %s; nullable: %t",
		prefix, relation.SchemaName, relation.Name, column.Name, column.TypeCategory, column.Nullable,
	)
	if len(column.AllowedValues) > 0 {
		fmt.Fprintf(rendered, "; allowed values: %s", strings.Join(column.AllowedValues, ", "))
	}
	rendered.WriteByte('\n')
}

func databaseQueryProjectedRelation(
	state DatabaseQueryIntentLeafState,
	id string,
) (datasource.IntentRelationProjection, bool) {
	for _, relation := range state.Authority.SchemaProjection.Relations {
		if relation.ID == id {
			return relation, true
		}
	}
	return datasource.IntentRelationProjection{}, false
}

func databaseQueryFilterOperators(
	state DatabaseQueryIntentLeafState,
	fieldID string,
) []datasource.FilterOperator {
	column, _, ok := databaseQueryColumn(state, fieldID)
	if !ok {
		return nil
	}
	operators := []datasource.FilterOperator{
		datasource.FilterEqual, datasource.FilterNotEqual,
		datasource.FilterIn, datasource.FilterNotIn,
		datasource.FilterIsNull, datasource.FilterIsNotNull,
	}
	switch column.TypeCategory {
	case datasource.TypeInteger, datasource.TypeDecimal, datasource.TypeTemporal, datasource.TypeDate:
		operators = append(operators, datasource.FilterGT, datasource.FilterGTE, datasource.FilterLT, datasource.FilterLTE)
	case datasource.TypeText:
		if len(column.AllowedValues) == 0 {
			operators = append(operators, datasource.FilterContains, datasource.FilterPrefix)
		}
	}
	return operators
}

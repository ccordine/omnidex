package assemblyline

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/datasource"
)

func renderDatabaseQueryAcceptedProjections(state DatabaseQueryIntentLeafState) (string, error) {
	var rendered strings.Builder
	fmt.Fprintf(&rendered, "ACCEPTED PROJECTIONS: %d\n", len(state.Projections))
	for _, projection := range state.Projections {
		semantic, err := databaseQueryProjectionSemantic(state, projection)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&rendered, "- %s\n", semantic)
	}
	return strings.TrimSpace(rendered.String()), nil
}

func renderDatabaseQueryProjectionCandidates(state DatabaseQueryIntentLeafState) (string, error) {
	used := make(map[int]struct{}, len(state.OrderBy))
	for _, term := range state.OrderBy {
		used[term.Projection] = struct{}{}
	}
	var rendered strings.Builder
	rendered.WriteString("SELECTABLE PROJECTIONS:\n")
	for index, projection := range state.Projections {
		if _, exists := used[index]; exists {
			continue
		}
		semantic, err := databaseQueryProjectionSemantic(state, projection)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&rendered, "PROJECTION %d %s\n", index, semantic)
	}
	return strings.TrimSpace(rendered.String()), nil
}

func renderDatabaseQueryFocusedProjection(
	state DatabaseQueryIntentLeafState,
	index int,
) (string, error) {
	if index < 0 || index >= len(state.Projections) {
		return "", fmt.Errorf("database query focused projection is outside accepted projections")
	}
	semantic, err := databaseQueryProjectionSemantic(state, state.Projections[index])
	if err != nil {
		return "", err
	}
	return "FOCUSED PROJECTION:\n" + semantic, nil
}

func databaseQueryProjectionSemantic(
	state DatabaseQueryIntentLeafState,
	projection datasource.RelationalProjection,
) (string, error) {
	if projection.Aggregate == datasource.AggregateCountRows {
		return "aggregate=count_rows", nil
	}
	field, err := databaseQueryFieldSemantic(state, projection.FieldID)
	if err != nil {
		return "", err
	}
	if projection.Aggregate != "" {
		return fmt.Sprintf("aggregate=%s field=%s", projection.Aggregate, field), nil
	}
	if projection.TimeBucket != "" {
		return fmt.Sprintf("time_bucket=%s field=%s", projection.TimeBucket, field), nil
	}
	return "field=" + field, nil
}

func renderDatabaseQueryAcceptedFilters(
	state DatabaseQueryIntentLeafState,
	filters []datasource.RelationalPredicate,
) (string, error) {
	var rendered strings.Builder
	fmt.Fprintf(&rendered, "ACCEPTED FILTERS: %d\n", len(filters))
	for _, predicate := range filters {
		semantic, err := databaseQueryPredicateSemantic(state, predicate)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&rendered, "- %s\n", semantic)
	}
	return strings.TrimSpace(rendered.String()), nil
}

func databaseQueryPredicateSemantic(
	state DatabaseQueryIntentLeafState,
	predicate datasource.RelationalPredicate,
) (string, error) {
	field, err := databaseQueryFieldSemantic(state, predicate.FieldID)
	if err != nil {
		return "", err
	}
	values := make([]string, len(predicate.Values))
	for index, value := range predicate.Values {
		values[index] = value.Value
	}
	semantic := fmt.Sprintf("field=%s operator=%s", field, predicate.Operator)
	if len(values) > 0 {
		semantic += " values=" + strings.Join(values, " | ")
	}
	return semantic, nil
}

func renderDatabaseQueryFilterScope(input DatabaseQueryFilterLeafInput) (string, error) {
	if input.ScopeRelationID == "" {
		return "", nil
	}
	return renderDatabaseQueryFocusedRelation(input.State, input.ScopeRelationID)
}

func renderDatabaseQueryFilterOperator(input DatabaseQueryFilterLeafInput) string {
	if input.Operator == "" {
		return ""
	}
	return "ACCEPTED FILTER OPERATOR:\n" + string(input.Operator)
}

func renderDatabaseQueryAllowedFilterOperators(input DatabaseQueryFilterLeafInput) string {
	operators := databaseQueryFilterOperators(input.State, input.FieldID)
	parts := make([]string, len(operators))
	for index, operator := range operators {
		parts[index] = string(operator)
	}
	return "SELECTABLE OPERATORS:\n" + strings.Join(parts, " | ")
}

func renderDatabaseQueryAcceptedValues(input DatabaseQueryFilterLeafInput) string {
	var rendered strings.Builder
	fmt.Fprintf(&rendered, "ACCEPTED VALUES: %d\n", len(input.AcceptedValues))
	for _, value := range input.AcceptedValues {
		fmt.Fprintf(&rendered, "- %s\n", value.Value)
	}
	return strings.TrimSpace(rendered.String())
}

func renderDatabaseQueryAcceptedWindows(state DatabaseQueryIntentLeafState) (string, error) {
	var rendered strings.Builder
	fmt.Fprintf(&rendered, "ACCEPTED TEMPORAL WINDOWS: %d\n", len(state.TemporalWindows))
	for _, window := range state.TemporalWindows {
		field, err := databaseQueryFieldSemantic(state, window.FieldID)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&rendered, "- field=%s unit=%s amount=%d\n", field, window.Unit, window.Amount)
	}
	return strings.TrimSpace(rendered.String()), nil
}

func renderDatabaseQueryAcceptedExistence(state DatabaseQueryIntentLeafState) (string, error) {
	var rendered strings.Builder
	fmt.Fprintf(&rendered, "ACCEPTED EXISTENCE PREDICATES: %d\n", len(state.Exists))
	for _, predicate := range state.Exists {
		relation, ok := databaseQueryProjectedRelation(state, predicate.RelationID)
		if !ok {
			return "", fmt.Errorf("database query accepted existence relation %q was not projected", predicate.RelationID)
		}
		meaning := "exists"
		if predicate.Negated {
			meaning = "does_not_exist"
		}
		fmt.Fprintf(&rendered, "- relation=%s.%s relation_requirement=%s\n", relation.SchemaName, relation.Name, meaning)
		for _, filter := range predicate.Filters {
			semantic, err := databaseQueryPredicateSemantic(state, filter)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&rendered, "  filter %s\n", semantic)
		}
	}
	return strings.TrimSpace(rendered.String()), nil
}

func renderDatabaseQueryAcceptedHaving(state DatabaseQueryIntentLeafState) (string, error) {
	var rendered strings.Builder
	fmt.Fprintf(&rendered, "ACCEPTED HAVING PREDICATES: %d\n", len(state.Having))
	for _, predicate := range state.Having {
		field := "rows"
		if predicate.Aggregate != datasource.AggregateCountRows {
			var err error
			field, err = databaseQueryFieldSemantic(state, predicate.FieldID)
			if err != nil {
				return "", err
			}
		}
		fmt.Fprintf(
			&rendered, "- aggregate=%s field=%s operator=%s value=%s\n",
			predicate.Aggregate, field, predicate.Operator, predicate.Value.Value,
		)
	}
	return strings.TrimSpace(rendered.String()), nil
}

func renderDatabaseQueryAcceptedOrder(state DatabaseQueryIntentLeafState) (string, error) {
	var rendered strings.Builder
	fmt.Fprintf(&rendered, "ACCEPTED ORDER TERMS: %d\n", len(state.OrderBy))
	for _, term := range state.OrderBy {
		if term.Projection < 0 || term.Projection >= len(state.Projections) {
			return "", fmt.Errorf("database query accepted order projection is outside accepted projections")
		}
		projection, err := databaseQueryProjectionSemantic(state, state.Projections[term.Projection])
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&rendered, "- %s direction=%s\n", projection, term.Direction)
	}
	return strings.TrimSpace(rendered.String()), nil
}

func databaseQueryFieldSemantic(state DatabaseQueryIntentLeafState, fieldID string) (string, error) {
	column, relationID, ok := databaseQueryColumn(state, fieldID)
	if !ok {
		return "", fmt.Errorf("database query semantic field %q was not projected", fieldID)
	}
	relation, ok := databaseQueryProjectedRelation(state, relationID)
	if !ok {
		return "", fmt.Errorf("database query semantic field relation %q was not projected", relationID)
	}
	return relation.SchemaName + "." + relation.Name + "." + column.Name, nil
}

func databaseQueryAggregateFieldEligible(
	aggregate datasource.AggregateOperation,
) func(datasource.IntentColumnProjection) bool {
	return func(column datasource.IntentColumnProjection) bool {
		switch aggregate {
		case "", datasource.AggregateCount, datasource.AggregateCountDistinct:
			return true
		case datasource.AggregateSum, datasource.AggregateAverage:
			return column.TypeCategory == datasource.TypeInteger || column.TypeCategory == datasource.TypeDecimal
		case datasource.AggregateMinimum, datasource.AggregateMaximum:
			if len(column.AllowedValues) > 0 {
				return false
			}
			switch column.TypeCategory {
			case datasource.TypeInteger, datasource.TypeDecimal, datasource.TypeText,
				datasource.TypeTemporal, datasource.TypeDate:
				return true
			}
		}
		return false
	}
}

func databaseQueryTemporalFieldEligible(column datasource.IntentColumnProjection) bool {
	return column.TypeCategory == datasource.TypeTemporal || column.TypeCategory == datasource.TypeDate
}

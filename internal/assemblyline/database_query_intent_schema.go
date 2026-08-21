package assemblyline

import "github.com/gryph/omnidex/internal/datasource"

func DatabaseQueryIntentResponseSchema(input DatabaseQueryIntentInput) (map[string]any, error) {
	if err := input.validate(); err != nil {
		return nil, err
	}
	relationIDs, columnIDs := databaseIntentSchemaIDs(input.SchemaProjection)
	projection := objectSchema([]string{"field_id", "aggregate", "time_bucket"}, map[string]any{
		"field_id":    enumStringSchema(append([]string{""}, columnIDs...)),
		"aggregate":   enumStringSchema([]string{"", "count_rows", "count", "count_distinct", "sum", "average", "minimum", "maximum"}),
		"time_bucket": enumStringSchema([]string{"", "day", "week", "month", "quarter", "year"}),
	})
	literal := objectSchema([]string{"type", "value"}, map[string]any{
		"type":  enumStringSchema([]string{"string", "integer", "decimal", "boolean", "timestamp", "date", "uuid"}),
		"value": map[string]any{"type": "string"},
	})
	filter := databaseIntentFilterSchema(columnIDs, literal)
	having := objectSchema([]string{"aggregate", "field_id", "operator", "value"}, map[string]any{
		"aggregate": enumStringSchema([]string{"count_rows", "count", "count_distinct", "sum", "average", "minimum", "maximum"}),
		"field_id":  enumStringSchema(append([]string{""}, columnIDs...)),
		"operator":  enumStringSchema([]string{"eq", "neq", "gt", "gte", "lt", "lte"}),
		"value":     literal,
	})
	return objectSchema([]string{
		"schema", "evidence_need_id", "from_relation_id", "shape", "projections", "filters",
		"temporal_windows", "exists", "group_by", "having", "order_by", "limit",
	}, map[string]any{
		"schema":           map[string]any{"type": "string", "const": DatabaseQueryIntentV1},
		"evidence_need_id": map[string]any{"type": "string", "const": input.EvidenceNeedID},
		"from_relation_id": enumStringSchema(relationIDs),
		"shape":            enumStringSchema([]string{"records", "scalar", "ranking", "distribution", "comparison", "trend", "existence"}),
		"projections":      map[string]any{"type": "array", "minItems": 0, "maxItems": datasource.MaxIntentProjections, "items": projection},
		"filters":          map[string]any{"type": "array", "minItems": 0, "maxItems": datasource.MaxIntentFilters, "items": filter},
		"temporal_windows": map[string]any{"type": "array", "minItems": 0, "maxItems": datasource.MaxIntentFilters, "items": databaseIntentWindowSchema(columnIDs)},
		"exists":           map[string]any{"type": "array", "minItems": 0, "maxItems": datasource.MaxIntentExistenceChecks, "items": databaseIntentExistenceSchema(relationIDs, filter)},
		"group_by":         map[string]any{"type": "array", "minItems": 0, "maxItems": datasource.MaxIntentGroups, "uniqueItems": true, "items": map[string]any{"type": "integer", "minimum": 0, "maximum": datasource.MaxIntentProjections - 1}},
		"having":           map[string]any{"type": "array", "minItems": 0, "maxItems": datasource.MaxIntentFilters, "items": having},
		"order_by":         map[string]any{"type": "array", "minItems": 0, "maxItems": datasource.MaxIntentOrderTerms, "items": databaseIntentOrderSchema()},
		"limit":            map[string]any{"type": "integer", "minimum": 1, "maximum": input.MaxRows},
	}), nil
}

func databaseIntentSchemaIDs(projection datasource.IntentSchemaProjection) ([]string, []string) {
	relations := make([]string, 0, len(projection.Relations))
	columns := make([]string, 0)
	for _, relation := range projection.Relations {
		relations = append(relations, relation.ID)
		for _, column := range relation.Columns {
			columns = append(columns, column.ID)
		}
	}
	return relations, columns
}

func databaseIntentFilterSchema(columnIDs []string, literal map[string]any) map[string]any {
	return objectSchema([]string{"field_id", "operator", "values"}, map[string]any{
		"field_id": enumStringSchema(columnIDs),
		"operator": enumStringSchema([]string{"eq", "neq", "gt", "gte", "lt", "lte", "in", "not_in", "is_null", "is_not_null", "contains", "prefix"}),
		"values":   map[string]any{"type": "array", "minItems": 0, "maxItems": datasource.MaxIntentFilterValues, "items": literal},
	})
}

func databaseIntentWindowSchema(columnIDs []string) map[string]any {
	return objectSchema([]string{"field_id", "unit", "amount"}, map[string]any{
		"field_id": enumStringSchema(columnIDs),
		"unit":     enumStringSchema([]string{"hour", "day", "week", "month", "year"}),
		"amount":   map[string]any{"type": "integer", "minimum": 1, "maximum": 10000},
	})
}

func databaseIntentExistenceSchema(relationIDs []string, filter map[string]any) map[string]any {
	return objectSchema([]string{"relation_id", "negated", "filters"}, map[string]any{
		"relation_id": enumStringSchema(relationIDs),
		"negated":     map[string]any{"type": "boolean"},
		"filters":     map[string]any{"type": "array", "minItems": 0, "maxItems": datasource.MaxIntentFilters, "items": filter},
	})
}

func databaseIntentOrderSchema() map[string]any {
	return objectSchema([]string{"projection", "direction"}, map[string]any{
		"projection": map[string]any{"type": "integer", "minimum": 0, "maximum": datasource.MaxIntentProjections - 1},
		"direction":  enumStringSchema([]string{"asc", "desc"}),
	})
}

func enumStringSchema(values []string) map[string]any {
	return map[string]any{"type": "string", "enum": values}
}

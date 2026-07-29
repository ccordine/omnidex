package assemblyline

func objectSchema(required []string, properties map[string]any) map[string]any {
	return map[string]any{
		"type": "object", "required": required, "properties": properties, "additionalProperties": false,
	}
}

func enumSchema[T ~string](values ...T) map[string]any {
	items := make([]string, 0, len(values))
	for _, value := range values {
		items = append(items, string(value))
	}
	return map[string]any{"type": "string", "enum": items}
}

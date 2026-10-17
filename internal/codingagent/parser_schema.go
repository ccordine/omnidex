package codingagent

func ParserOutputSchema(call Assignment) map[string]any {
	if call.ParserStep != nil {
		return parserStepSchema(call)
	}
	if call.Role == Execute || call.Role == Plan || call.Role == Review {
		return nil // Ordinary Codex conversation, not a structured-output station.
	}
	text := map[string]any{"type": "string"}
	strings := map[string]any{"type": "array", "items": text}
	object := func(properties map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	}
	if call.Role == PlanningInitial {
		return planningInitialOutputSchema(object, text, strings)
	}
	if call.Role == PlanningMutation {
		return planningMutationOutputSchema(object, text, strings)
	}
	if call.Role == ExecutionResponse {
		return object(map[string]any{"kind": planningEnum("done", "question", "permission", "error")}, "kind")
	}
	variants := []map[string]any{}
	for _, task := range ReviewReferences(call) {
		criteria := make([]string, len(task.Criteria))
		for index, criterion := range task.Criteria {
			criteria[index] = criterion.Reference
		}
		variants = append(variants, object(map[string]any{
			"file": text, "requirement": map[string]any{"type": "string", "enum": criteria}, "observation": text,
		}, "file", "requirement", "observation"))
	}
	finding := map[string]any{"anyOf": variants}
	if call.Role == ReviewResponse && len(variants) == 1 {
		finding = variants[0]
	}
	return object(map[string]any{"kind": planningEnum("done", "question", "permission", "error"), "findings": map[string]any{"type": "array", "items": finding}, "verification": text}, "kind", "findings", "verification")
}

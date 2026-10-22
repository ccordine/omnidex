package codingagent

func ParserOutputSchema(call Assignment) map[string]any {
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
	finding := object(map[string]any{"file": text, "observation": text}, "file", "observation")
	return object(map[string]any{"kind": planningEnum("done", "question", "permission", "error"), "findings": map[string]any{"type": "array", "items": finding}, "verification": text}, "kind", "findings", "verification")
}

package codex

func planningEnum(values ...string) map[string]any {
	return map[string]any{"type": "string", "enum": values}
}

// Planning stations use their own native contracts. A native provider emits
// exactly the value decoded by the station that invoked it.
func planningInitialOutputSchema(
	object func(map[string]any, ...string) map[string]any,
	text, strings map[string]any,
) map[string]any {
	ledger := planningLedgerSchema(object, text, strings)
	properties := ledger["properties"].(map[string]any)
	for _, collection := range []string{"acceptance_criteria", "checks"} {
		item := properties[collection].(map[string]any)["items"].(map[string]any)
		item["properties"].(map[string]any)["required"] = map[string]any{"type": "boolean", "enum": []bool{true}}
	}
	return object(map[string]any{
		"ledger":                 ledger,
		"owner_turn_disposition": planningDispositionSchema(object, text, strings),
	}, "ledger", "owner_turn_disposition")
}

func planningMutationOutputSchema(
	object func(map[string]any, ...string) map[string]any,
	text, strings map[string]any,
) map[string]any {
	integer := map[string]any{"type": "integer"}
	nullableInteger := map[string]any{"anyOf": []map[string]any{integer, {"type": "null"}}}
	nullableItem := map[string]any{"anyOf": []map[string]any{planningMutationItemSchema(object, text, strings), {"type": "null"}}}
	operation := object(map[string]any{
		"kind":      planningEnum("add", "amend", "remove", "restore", "set_review_state", "reorder"),
		"target_id": text, "item": nullableItem,
		"review_state": planningEnum("", "proposed", "agreed", "needs_discussion"),
		"rank":         nullableInteger, "rationale": text,
	}, "kind", "target_id", "item", "review_state", "rank", "rationale")
	return object(map[string]any{
		"operation_id": text, "base_revision": integer, "owner_turn_id": text,
		"owner_turn_disposition": planningDispositionSchema(object, text, strings),
		"operations":             map[string]any{"type": "array", "items": operation},
	}, "operation_id", "base_revision", "owner_turn_id", "owner_turn_disposition", "operations")
}

func planningDispositionSchema(
	object func(map[string]any, ...string) map[string]any,
	text, strings map[string]any,
) map[string]any {
	statement := object(map[string]any{
		"kind":     planningEnum("represented", "out_of_scope", "unresolved"),
		"item_ids": strings, "reason": text, "question_ids": strings,
	}, "kind", "item_ids", "reason", "question_ids")
	statement["anyOf"] = planningDispositionVariants(text)
	result := object(map[string]any{
		"kind":     planningEnum("represented", "out_of_scope", "unresolved"),
		"item_ids": strings, "reason": text, "question_ids": strings,
		"statements": map[string]any{"type": "array", "items": statement},
	}, "kind", "item_ids", "reason", "question_ids", "statements")
	result["anyOf"] = planningDispositionVariants(text)
	return result
}

func planningDispositionVariants(text map[string]any) []map[string]any {
	emptyItems := map[string]any{"type": "array", "items": text, "maxItems": 0}
	nonEmptyItems := map[string]any{"type": "array", "items": text, "minItems": 1}
	empty := map[string]any{"type": "string", "enum": []string{""}}
	return []map[string]any{
		{"properties": map[string]any{
			"kind": planningEnum("represented"), "item_ids": nonEmptyItems,
			"reason": empty, "question_ids": emptyItems,
		}},
		{"properties": map[string]any{
			"kind": planningEnum("out_of_scope"), "item_ids": emptyItems,
			"reason": map[string]any{"type": "string", "minLength": 1}, "question_ids": emptyItems,
		}},
		{"properties": map[string]any{
			"kind": planningEnum("unresolved"), "item_ids": emptyItems,
			"reason": empty, "question_ids": nonEmptyItems,
		}},
	}
}

func planningLedgerSchema(
	object func(map[string]any, ...string) map[string]any,
	text, strings map[string]any,
) map[string]any {
	integer := map[string]any{"type": "integer"}
	boolean := map[string]any{"type": "boolean"}
	base := func(extra map[string]any, required ...string) map[string]any {
		properties := map[string]any{
			"id": text, "provenance": object(map[string]any{"kind": text, "reference": text, "revision": integer}, "kind", "reference", "revision"),
			"rationale": text, "priority": planningEnum("critical", "high", "normal", "low"), "rank": integer,
			"disposition":  planningEnum("active", "tombstoned"),
			"review_state": planningEnum("", "proposed", "agreed", "needs_discussion"),
		}
		for name, schema := range extra {
			properties[name] = schema
		}
		return object(properties, append([]string{"id", "provenance", "rationale", "priority", "rank", "disposition", "review_state"}, required...)...)
	}
	array := func(item map[string]any) map[string]any { return map[string]any{"type": "array", "items": item} }
	objective := base(map[string]any{"outcome": text}, "outcome")
	requirement := base(map[string]any{"outcome": text}, "outcome")
	constraint := base(map[string]any{"rule": text}, "rule")
	group := base(map[string]any{"title": text, "parent_group_id": text}, "title", "parent_group_id")
	task := base(map[string]any{"goal": text, "group_id": text, "requirement_ids": strings,
		"constraint_ids": strings, "acceptance_criterion_ids": strings, "delivery_obligation": text,
		"known_affected_systems": strings, "read_files": strings, "write_files": strings, "delete_files": strings},
		"goal", "group_id", "requirement_ids", "constraint_ids", "acceptance_criterion_ids",
		"delivery_obligation", "known_affected_systems", "read_files", "write_files", "delete_files")
	criterion := base(map[string]any{"outcome": text, "requirement_ids": strings, "task_ids": strings, "required": boolean},
		"outcome", "requirement_ids", "task_ids", "required")
	check := base(map[string]any{"argv": strings, "observation": text, "requirement_ids": strings,
		"acceptance_criterion_ids": strings, "required": boolean}, "argv", "observation", "requirement_ids", "acceptance_criterion_ids", "required")
	check["anyOf"] = []map[string]any{
		{"properties": map[string]any{
			"argv":        map[string]any{"type": "array", "items": text, "minItems": 1},
			"observation": map[string]any{"type": "string", "enum": []string{""}},
		}},
		{"properties": map[string]any{
			"argv":        map[string]any{"type": "array", "items": text, "maxItems": 0},
			"observation": map[string]any{"type": "string", "minLength": 1},
		}},
	}
	decision := base(map[string]any{"kind": planningEnum("decision", "question"), "prompt": text, "answer": text, "affects": strings}, "kind", "prompt", "answer", "affects")
	dependency := base(map[string]any{"task_id": text, "prerequisite_id": text, "kind": planningEnum("requires", "blocks")}, "task_id", "prerequisite_id", "kind")
	finding := base(map[string]any{"kind": planningEnum("planning"), "observation": text, "target_ids": strings, "resolved_by": strings}, "kind", "observation", "target_ids", "resolved_by")
	return object(map[string]any{
		"id": text, "revision": integer, "objectives": array(objective), "requirements": array(requirement),
		"constraints": array(constraint), "groups": array(group), "tasks": array(task),
		"acceptance_criteria": array(criterion), "checks": array(check),
		"decisions_and_questions": array(decision), "dependencies": array(dependency), "findings": array(finding),
	}, "id", "revision", "objectives", "requirements", "constraints", "groups", "tasks", "acceptance_criteria", "checks", "decisions_and_questions", "dependencies", "findings")
}

func planningMutationItemSchema(
	object func(map[string]any, ...string) map[string]any,
	text, strings map[string]any,
) map[string]any {
	ledger := planningLedgerSchema(object, text, strings)
	properties := ledger["properties"].(map[string]any)
	names := []string{"objective", "requirement", "constraint", "group", "task", "acceptance_criterion", "check", "decision_question", "dependency", "finding"}
	sources := []string{"objectives", "requirements", "constraints", "groups", "tasks", "acceptance_criteria", "checks", "decisions_and_questions", "dependencies", "findings"}
	variants := make([]map[string]any, 0, len(names))
	for index, name := range names {
		item := properties[sources[index]].(map[string]any)["items"]
		variants = append(variants, object(map[string]any{name: item}, name))
	}
	return map[string]any{"anyOf": variants}
}

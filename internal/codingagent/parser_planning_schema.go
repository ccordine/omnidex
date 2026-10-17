package codingagent

import "sort"

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
	return object(map[string]any{
		"ledger": ledger,
	}, "ledger")
}

func planningMutationOutputSchema(
	object func(map[string]any, ...string) map[string]any,
	text, strings map[string]any,
) map[string]any {
	target := map[string]any{"type": "string", "pattern": `^(.|\n|\r)*[^\t\n\r\u000C ](.|\n|\r)*$`}
	variants := []map[string]any{
		object(map[string]any{"kind": planningEnum("add"), "item": planningMutationItemSchema(object, text, strings), "rationale": text}, "kind", "item", "rationale"),
		object(map[string]any{"kind": planningEnum("amend"), "target_id": target, "item": planningAmendItemSchema(object, text, strings), "rationale": text}, "kind", "target_id", "item", "rationale"),
		object(map[string]any{"kind": planningEnum("remove", "restore"), "target_id": target, "rationale": text}, "kind", "target_id", "rationale"),
		object(map[string]any{"kind": planningEnum("set_review_state"), "target_id": target, "review_state": planningEnum("proposed", "agreed", "needs_discussion"), "rationale": text}, "kind", "target_id", "review_state", "rationale"),
		object(map[string]any{"kind": planningEnum("reorder"), "target_id": target, "rank": map[string]any{"type": "integer", "minimum": 0}, "rationale": text}, "kind", "target_id", "rank", "rationale"),
	}
	operation := map[string]any{"anyOf": variants}
	return object(map[string]any{
		"operations": map[string]any{"type": "array", "items": operation},
	}, "operations")
}

// An amendment contains semantic changes only. Null means unchanged; explicit
// empty text/arrays mean clearing that field and still face domain validation.
// Identity, provenance, lifecycle and order stay code-owned here.
func planningAmendItemSchema(
	object func(map[string]any, ...string) map[string]any,
	text, strings map[string]any,
) map[string]any {
	schema := planningMutationItemSchema(object, text, strings)
	for _, variant := range schema["anyOf"].([]map[string]any) {
		for _, payload := range variant["properties"].(map[string]any) {
			item := payload.(map[string]any)
			properties := item["properties"].(map[string]any)
			for _, name := range []string{"id", "provenance", "disposition", "review_state", "rank"} {
				delete(properties, name)
			}
			required := []string{}
			for name, value := range properties {
				properties[name] = map[string]any{"anyOf": []any{value, map[string]any{"type": "null"}}}
				required = append(required, name)
			}
			sort.Strings(required)
			item["required"] = required
			delete(item, "anyOf")
		}
	}
	return schema
}

func planningLedgerSchema(
	object func(map[string]any, ...string) map[string]any,
	text, strings map[string]any,
) map[string]any {
	integer := map[string]any{"type": "integer"}
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
	criterion := base(map[string]any{"outcome": text, "requirement_ids": strings, "task_ids": strings},
		"outcome", "requirement_ids", "task_ids")
	check := base(map[string]any{"argv": strings, "observation": text, "requirement_ids": strings,
		"acceptance_criterion_ids": strings}, "argv", "observation", "requirement_ids", "acceptance_criterion_ids")
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
	dependency := base(map[string]any{"task_id": text, "prerequisite_id": text, "kind": planningEnum("requires", "blocks")}, "task_id", "prerequisite_id", "kind")
	return object(map[string]any{
		"id": text, "revision": integer, "objectives": array(objective), "requirements": array(requirement),
		"constraints": array(constraint), "groups": array(group), "tasks": array(task),
		"acceptance_criteria": array(criterion), "checks": array(check),
		"dependencies": array(dependency),
	}, "id", "revision", "objectives", "requirements", "constraints", "groups", "tasks", "acceptance_criteria", "checks", "dependencies")
}

func planningMutationItemSchema(
	object func(map[string]any, ...string) map[string]any,
	text, strings map[string]any,
) map[string]any {
	ledger := planningLedgerSchema(object, text, strings)
	properties := ledger["properties"].(map[string]any)
	names := []string{"objective", "requirement", "constraint", "group", "task", "acceptance_criterion", "check", "dependency"}
	sources := []string{"objectives", "requirements", "constraints", "groups", "tasks", "acceptance_criteria", "checks", "dependencies"}
	variants := make([]map[string]any, 0, len(names))
	for index, name := range names {
		item := properties[sources[index]].(map[string]any)["items"]
		variants = append(variants, object(map[string]any{name: item}, name))
	}
	return map[string]any{"anyOf": variants}
}

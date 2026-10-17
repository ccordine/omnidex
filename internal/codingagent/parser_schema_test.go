package codingagent

import (
	"reflect"
	"testing"
)

func TestPlanningSchemaConstrainsCodeOwnedLedgerEnums(t *testing.T) {
	text := map[string]any{"type": "string"}
	strings := map[string]any{"type": "array", "items": text}
	object := func(properties map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	}
	ledger := planningLedgerSchema(object, text, strings)
	properties := ledger["properties"].(map[string]any)
	for _, name := range []string{"acceptance_criteria", "checks"} {
		item := properties[name].(map[string]any)["items"].(map[string]any)
		if item["properties"].(map[string]any)["required"] != nil {
			t.Fatal("parser schema asks AI to decide whether planned verification is required")
		}
	}
	objective := properties["objectives"].(map[string]any)["items"].(map[string]any)
	item := objective["properties"].(map[string]any)
	for name, expected := range map[string][]string{
		"priority":     {"critical", "high", "normal", "low"},
		"disposition":  {"active", "tombstoned"},
		"review_state": {"", "proposed", "agreed", "needs_discussion"},
	} {
		actual, ok := item[name].(map[string]any)["enum"].([]string)
		if !ok || !reflect.DeepEqual(actual, expected) {
			t.Fatalf("planning %s enum = %#v, want %#v", name, actual, expected)
		}
	}
	check := properties["checks"].(map[string]any)["items"].(map[string]any)
	if variants, ok := check["anyOf"].([]map[string]any); !ok || len(variants) != 2 {
		t.Fatalf("planning check schema does not require exactly one command or observation: %#v", check)
	}
	for _, variant := range check["anyOf"].([]map[string]any) {
		if !reflect.DeepEqual(variant["required"], check["required"]) || variant["additionalProperties"] != false {
			t.Fatalf("check variant dropped required object fields: %#v", variant)
		}
		for name, schema := range check["properties"].(map[string]any) {
			if name != "argv" && name != "observation" && !reflect.DeepEqual(variant["properties"].(map[string]any)[name], schema) {
				t.Fatalf("check variant dropped %s contract", name)
			}
		}
	}
	for _, forbidden := range []string{"decisions", "findings", "questions"} {
		if _, exists := properties[forbidden]; exists {
			t.Fatalf("planning schema retained model-authored routing bucket %q", forbidden)
		}
	}
}

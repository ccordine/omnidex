package api

import (
	"testing"

	"github.com/gryph/omnidex/internal/modelconfig"
)

func TestDecodeUIProjectModelFieldsRequiresRegisteredTypedInventory(t *testing.T) {
	raw := modelconfig.Config{}.FieldList(map[string]string{})
	fields, err := decodeUIProjectModelFields(map[string]any{"fields": raw})
	if err != nil || len(fields) != len(modelconfig.Fields) {
		t.Fatalf("fields=%d err=%v", len(fields), err)
	}
	for name, mutate := range map[string]func([]map[string]any){
		"missing field":          func(fields []map[string]any) { delete(fields[0], "label") },
		"unknown field":          func(fields []map[string]any) { fields[0]["agent"] = "forbidden" },
		"wrong key":              func(fields []map[string]any) { fields[0]["key"] = "coding_fragment_agent" },
		"wrong options type":     func(fields []map[string]any) { fields[0]["options"] = []any{} },
		"inexact resolved value": func(fields []map[string]any) { fields[0]["value"] = " padded " },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := cloneUIProjectModelFields(raw)
			mutate(candidate)
			if _, err := decodeUIProjectModelFields(map[string]any{"fields": candidate}); err == nil {
				t.Fatalf("invalid field inventory was accepted: %#v", candidate[0])
			}
		})
	}
}

func cloneUIProjectModelFields(fields []map[string]any) []map[string]any {
	out := make([]map[string]any, len(fields))
	for index, field := range fields {
		out[index] = make(map[string]any, len(field))
		for key, value := range field {
			out[index][key] = value
		}
	}
	return out
}

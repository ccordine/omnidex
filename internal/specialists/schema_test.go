package specialists

import (
	"strings"
	"testing"
)

func TestValidatePayloadAgainstSchemaEnforcesArrayBoundsAndUniqueness(t *testing.T) {
	schema := []byte(`{
		"type":"object",
		"required":["items"],
		"properties":{
			"items":{"type":"array","minItems":1,"maxItems":2,"uniqueItems":true,"items":{"type":"string"}}
		},
		"additionalProperties":false
	}`)
	tests := []struct {
		name    string
		items   []string
		wantErr string
	}{
		{name: "minimum", items: []string{}, wantErr: "at least 1"},
		{name: "maximum", items: []string{"one", "two", "three"}, wantErr: "at most 2"},
		{name: "unique", items: []string{"same", "same"}, wantErr: "unique"},
		{name: "valid", items: []string{"one", "two"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidatePayloadAgainstSchema(schema, map[string]any{"items": test.items})
			if test.wantErr == "" && err != nil {
				t.Fatalf("valid payload rejected: %v", err)
			}
			if test.wantErr != "" && (err == nil || !strings.Contains(err.Error(), test.wantErr)) {
				t.Fatalf("err=%v, want %q", err, test.wantErr)
			}
		})
	}
}

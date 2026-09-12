package evidence

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestEvidenceDecodeUsesActualFieldsAndRejectsRetiredHash(t *testing.T) {
	t.Parallel()
	for _, original := range []Record{
		{Kind: KindObjectiveCitation, SourceRef: "https://source.example/document", Excerpt: "An observed source passage."},
		{Kind: KindCommandOutput, Command: "go test ./...", Excerpt: "ok example/package", Metadata: map[string]any{"exit_code": float64(0)}},
	} {
		raw, err := json.Marshal(original)
		if err != nil {
			t.Fatal(err)
		}
		var decoded Record
		if err := json.Unmarshal(raw, &decoded); err != nil || !reflect.DeepEqual(decoded, original) {
			t.Fatalf("actual evidence did not round-trip: %#v / %v", decoded, err)
		}
	}
	for _, raw := range []string{
		`{"kind":"objective_citation","hash":"anything"}`,
		`{"kind":"command_output","unknown_authority":true}`,
		`{"Kind":"command_output"}`,
		`{"kind":"command_output","kind":"objective_citation"}`,
		`null`, `[]`, `{}`, `{"kind":"command_output"} {}`,
	} {
		var record Record
		if err := json.Unmarshal([]byte(raw), &record); err == nil {
			t.Errorf("invalid evidence was accepted: %s", raw)
		}
	}
}

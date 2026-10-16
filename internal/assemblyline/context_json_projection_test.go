package assemblyline

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestContextJSONProjectionRetainsDecodedValuesWithoutSerializationEscapes(t *testing.T) {
	nested := `{"id":"task-1","rank":9007199254740993,"goal":"Keep the note.\nNo changes outside scope.","read_files":[]}`
	raw, _ := json.Marshal(map[string]any{"quote": nested, "required": false, "nothing": nil, "paths": []string{`C:\work\notes.txt`, "/opt/project/settings.toml"}})
	redacted, bindings, err := RedactContextArtifactJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := ProjectContextJSONText([]byte(redacted))
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"context.quote.id:\ntask-1", "context.quote.rank: 9007199254740993", "Keep the note.\nNo changes outside scope.", "context.quote.read_files: []", "context.required: false", "context.nothing: null"} {
		if !strings.Contains(projected, value) {
			t.Fatalf("projection lost exact leaf %q: %s", value, projected)
		}
	}
	if strings.Contains(projected, `\"`) || strings.Contains(projected, `\n`) {
		t.Fatal("serialization escapes survived decoded projection")
	}
	restored, err := RestoreContextArtifactJSONOrText(projected, bindings)
	if err != nil || !strings.Contains(restored, `C:\work\notes.txt`) || !strings.Contains(restored, "/opt/project/settings.toml") {
		t.Fatalf("real artifact bindings did not survive: %s %v", restored, err)
	}
}

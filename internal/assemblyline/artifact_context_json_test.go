package assemblyline

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestCapturedPlanningContextDoesNotRedactJSONEscapesAsPaths(t *testing.T) {
	for _, name := range []string{"expense", "wordcounter"} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile("../../testdata/planning-gauntlet/" + name + ".json")
			if err != nil {
				t.Fatal(err)
			}
			var fixture struct {
				Context json.RawMessage `json:"mutation_parser_context"`
			}
			if err := json.Unmarshal(raw, &fixture); err != nil {
				t.Fatal(err)
			}
			redacted, bindings, err := RedactContextArtifactJSON(fixture.Context)
			if err != nil {
				t.Fatal(err)
			}
			for _, binding := range bindings {
				if strings.Contains(binding.Value, `\"`) || strings.Contains(binding.Value, `\n`) {
					t.Fatalf("JSON syntax was redacted as a filesystem identity: %+v", binding)
				}
			}
			if name == "expense" && !strings.Contains(redacted, "amount, and note") {
				t.Fatal("the real expense note obligation was hidden again")
			}
			restored, err := RestoreContextArtifactJSONOrText(redacted, bindings)
			if err != nil {
				t.Fatal(err)
			}
			identity := func(s string) (string, error) { return s, nil }
			original, err := mapContextJSON(fixture.Context, identity)
			if err != nil {
				t.Fatal(err)
			}
			if restored != string(original) {
				t.Fatal("structured ledger context changed during redaction/restoration")
			}
		})
	}
}

func TestContextJSONKeepsRealPathsAndNestedJSONByteValues(t *testing.T) {
	nested, _ := json.Marshal(map[string]string{"note": "date, category, amount, and note", "file": `C:\work\notes.txt`})
	input := map[string]string{"nested": string(nested), "path": "/opt/app/settings.toml", "same": `C:\work\notes.txt`, "output": "valid\n"}
	raw, _ := json.Marshal(input)
	redacted, bindings, err := RedactContextArtifactJSON(raw)
	if err != nil || len(bindings) != 2 || strings.Contains(redacted, "/opt/app") {
		t.Fatalf("redaction: %s %+v %v", redacted, bindings, err)
	}
	restored, err := RestoreContextArtifactJSONOrText(redacted, bindings)
	if err != nil {
		t.Fatal(err)
	}
	var output map[string]string
	if err := json.Unmarshal([]byte(restored), &output); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(input, output) {
		t.Fatalf("path or escaped byte changed: %#v", output)
	}
	if _, err := RestoreContextArtifactJSONOrText(`{"path":"ARTIFACT_99"}`, bindings); err == nil {
		t.Fatal("unknown binding accepted")
	}
}

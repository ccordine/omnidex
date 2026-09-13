package assemblyline

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoDocumentAssemblesAndExecutesStandardLibraryReferences(t *testing.T) {
	for _, fixture := range []struct{ name, signature, body, call, want, importPath string }{
		{"numeric magnitude", "func Value(input float64) float64", "// Measure the magnitude.\nreturn math.Abs(input)", "Value(-2.5)", "2.5", "math"},
		{"binary encoding", "func Value(input []byte) string", "/* Encode the bytes. */\nreturn hex.EncodeToString(input)", "Value([]byte{0, 127, 255})", `"007fff"`, "encoding/hex"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			document := SourceDocument{ID: "feature", Path: "value.go", Preamble: "package fixture", Blocks: []SourceBlock{{
				ID: "value", Signature: fixture.signature, Contract: fixture.name, API: fixture.signature,
			}}}
			composed, err := ComposeGoDocument(document, SourceComposition{
				Generated: map[string]string{"value": fixture.signature + " {\n" + fixture.body + "\n}"}, Interfaces: map[string]string{},
			})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(composed.Source, `"`+fixture.importPath+`"`) {
				t.Fatalf("missing derived import: %s", composed.Source)
			}
			lines := strings.Split(composed.Source, "\n")
			if !strings.HasPrefix(lines[composed.Spans["value"].StartLine-1], fixture.signature) {
				t.Fatalf("import insertion invalidated source spans: %+v", composed.Spans)
			}
			root := t.TempDir()
			files := map[string]string{
				"go.mod": "module fixture\n\ngo 1.24.0\n", "value.go": composed.Source,
				"value_test.go": "package fixture\nimport \"testing\"\nfunc TestValue(t *testing.T) { if got := " + fixture.call + "; got != " + fixture.want + " { t.Fatalf(\"unexpected value: %v\", got) } }",
			}
			for name, content := range files {
				if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			command := exec.CommandContext(t.Context(), "go", "test", "-count=1", "./...")
			command.Dir = root
			command.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off", "GOTOOLCHAIN=local")
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("execute composed fixture: %v\n%s", err, output)
			}
		})
	}
}

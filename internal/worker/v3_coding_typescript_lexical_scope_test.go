package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTypeScriptCompilerScopeInspectorFindsOnlyBindingsAtFailure(t *testing.T) {
	if os.Getenv("OMNIDEX_NODE_INTEGRATION") != "1" {
		t.Skip("set OMNIDEX_NODE_INTEGRATION=1 to install the pinned TypeScript compiler")
	}
	fixtures := []struct {
		name, source, marker string
		want, forbidden      []string
	}{
		{
			name: "inventory updater",
			source: `interface InventoryActions { commit(index: number, value: number): void }
function UpdateInventory(index: number, actions: InventoryActions): void {
  const values: number[] = [index];
  values.forEach(previousValue => {
    const nextValue = previousValue + 1;
    void nextValue;
  });
  actions.commit(index, nextValue);
}`,
			marker: "nextValue);",
			want: []string{
				"UpdateInventory: (index: number, actions: InventoryActions) => void", "actions: InventoryActions",
				"commit: (index: number, value: number) => void", "index: number", "values: number[]",
			},
			forbidden: []string{"nextValue", "previousValue"},
		},
		{
			name: "schedule updater",
			source: `interface ScheduleActions { move(key: string, day: number): void }
function MoveAppointment(day: number, actions: ScheduleActions): void {
  const entries: readonly number[] = [day];
  entries.map(previousDay => {
    const nextDay = previousDay + 1;
    return nextDay;
  });
  actions.move("primary", nextDay);
}`,
			marker: "nextDay);",
			want: []string{
				"MoveAppointment: (day: number, actions: ScheduleActions) => void", "actions: ScheduleActions",
				"day: number", "entries: readonly number[]", "move: (key: string, day: number) => void",
			},
			forbidden: []string{"nextDay", "previousDay"},
		},
	}
	root := t.TempDir()
	writeTypeScriptLexicalScopeFixtureProject(t, root)
	output, err := runDirectCodingStageCommand(
		context.Background(), root, directCodingTypeScriptInstallTimeout,
		"npm", directCodingNPMInstallArgs()...,
	)
	if err != nil {
		t.Fatalf("install pinned TypeScript compiler: %v\n%s", err, output)
	}
	if err := writeDirectCodingTypeScriptScopeInspector(root); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(root, "src/fixture.ts"), []byte(fixture.source), 0o600); err != nil {
				t.Fatal(err)
			}
			line, column := typeScriptLexicalScopeTestLocation(t, fixture.source, fixture.marker)
			scope, err := inspectDirectCodingTypeScriptScope(
				context.Background(), root,
				directCodingStageDiagnostic{
					CompilerIssue: true, DocumentPath: "src/fixture.ts",
					DocumentLine: line, DocumentColumn: column,
					DocumentBlockStartLine: 2, DocumentBlockEndLine: strings.Count(fixture.source, "\n") + 1,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			rendered := renderTypeScriptRepairBindingsForTest(scope.Bindings)
			unavailable := renderTypeScriptRepairBindingsForTest(scope.UnavailableBindings)
			for _, value := range fixture.want {
				if !strings.Contains(rendered, value) {
					t.Fatalf("bindings omitted %q:\n%s", value, rendered)
				}
			}
			for _, value := range fixture.forbidden {
				if strings.Contains(rendered, value) {
					t.Fatalf("bindings exposed out-of-scope %q:\n%s", value, rendered)
				}
				if !strings.Contains(unavailable, value) {
					t.Fatalf("unavailable bindings omitted %q:\n%s", value, unavailable)
				}
			}
		})
	}
}

func writeTypeScriptLexicalScopeFixtureProject(t *testing.T, root string) {
	t.Helper()
	for _, file := range typeScriptBrowserStaticFiles("scope-fixture", "Scope fixture", "") {
		if file.Path != "package.json" && file.Path != "tsconfig.json" {
			continue
		}
		if err := os.WriteFile(filepath.Join(root, file.Path), []byte(file.Content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
}

func typeScriptLexicalScopeTestLocation(t *testing.T, source, marker string) (int, int) {
	t.Helper()
	offset := strings.Index(source, marker)
	if offset < 0 {
		t.Fatalf("source lacks marker %q", marker)
	}
	prefix := source[:offset]
	return strings.Count(prefix, "\n") + 1, offset - strings.LastIndex(prefix, "\n")
}

func renderTypeScriptRepairBindingsForTest(bindings []assemblyline.TypeScriptRepairBinding) string {
	var lines []string
	for _, binding := range bindings {
		lines = append(lines, binding.Name+": "+binding.Type)
		lines = append(lines, binding.Members...)
	}
	return strings.Join(lines, "\n")
}

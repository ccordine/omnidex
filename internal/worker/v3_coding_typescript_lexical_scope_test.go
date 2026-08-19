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
		wantEvidence         []string
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
		{
			name: "regular expression escape in exact expression source",
			source: `function VerifyPattern(): void {
  const matches = (value: string) => value.includes('\\d+');
  void matches;
}`,
			marker: "includes('\\d+')",
			want: []string{
				"VerifyPattern: () => void", "matches: (value: string) => boolean",
			},
			wantEvidence: []string{`value.includes('\\d+')`},
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
			evidence := renderTypeScriptRepairExpressionEvidenceForTest(scope.ExpressionEvidence)
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
			for _, value := range fixture.wantEvidence {
				if !strings.Contains(evidence, value) {
					t.Fatalf("expression evidence omitted %q:\n%s", value, evidence)
				}
			}
		})
	}
}

func TestTypeScriptCompilerScopeInspectorProjectsRealUnionMismatch(t *testing.T) {
	if os.Getenv("OMNIDEX_NODE_INTEGRATION") != "1" {
		t.Skip("set OMNIDEX_NODE_INTEGRATION=1 to install the pinned TypeScript compiler")
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
	for _, testCase := range []struct {
		name, source, marker, wantExpression, wantBinding, wantReplacement string
	}{
		{
			name: "numeric hook state",
			source: `import { useState } from 'react';
type MetricValue = null | boolean | number | string;
type MetricState = { readonly [key: string]: MetricValue };
function ReadMetric(state: MetricState): number {
  const [reading] = useState<number>(() => state.reading ?? 0);
  return reading;
}`,
			marker: "() => state.reading", wantExpression: "state.reading ?? 0", wantBinding: "state",
			wantReplacement: "typeof state.reading === 'number' ? state.reading : 0",
		},
		{
			name: "text hook state",
			source: `import { useState } from 'react';
type LabelValue = null | boolean | number | string;
type LabelState = { readonly [key: string]: LabelValue };
function ReadLabel(state: LabelState): string {
  const [label] = useState<string>(() => state.label ?? '');
  return label;
}`,
			marker: "() => state.label", wantExpression: "state.label ?? ''", wantBinding: "state",
			wantReplacement: "typeof state.label === 'string' ? state.label : ''",
		},
		{
			name: "numeric reading",
			source: `type MetricValue = null | boolean | number | string;
interface MetricState { readonly reading: MetricValue }
function ReadMetric(state: MetricState): number {
  const reading: number = state.reading ?? 0;
  return reading;
}`,
			marker: "state.reading ?? 0", wantExpression: "state.reading ?? 0", wantBinding: "state",
			wantReplacement: "typeof state.reading === 'number' ? state.reading : 0",
		},
		{
			name: "text label",
			source: `type LabelValue = null | number | string;
interface LabelState { readonly label: LabelValue }
function ReadLabel(entry: LabelState): string {
  const label: string = entry.label ?? '';
  return label;
}`,
			marker: "entry.label ?? ''", wantExpression: "entry.label ?? ''", wantBinding: "entry",
			wantReplacement: "typeof entry.label === 'string' ? entry.label : ''",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(root, "src/fixture.ts"), []byte(testCase.source), 0o600); err != nil {
				t.Fatal(err)
			}
			line, column := typeScriptLexicalScopeTestLocation(t, testCase.source, testCase.marker)
			functionLine, _ := typeScriptLexicalScopeTestLocation(t, testCase.source, "function ")
			scope, err := inspectDirectCodingTypeScriptScope(
				context.Background(), root,
				directCodingStageDiagnostic{
					CompilerIssue: true, DocumentPath: "src/fixture.ts",
					DocumentLine: line, DocumentColumn: column,
					DocumentBlockStartLine: functionLine,
					DocumentBlockEndLine:   strings.Count(testCase.source, "\n") + 1,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if len(scope.ExpressionEvidence) != 1 ||
				scope.ExpressionEvidence[0].Source != testCase.wantExpression {
				t.Fatalf("compiler expression projection=%+v", scope.ExpressionEvidence)
			}
			if len(scope.Bindings) != 1 || scope.Bindings[0].Name != testCase.wantBinding {
				t.Fatalf("compiler binding projection=%+v", scope.Bindings)
			}
			if len(scope.DeterministicRepairs) != 1 ||
				scope.DeterministicRepairs[0].Replacement != testCase.wantReplacement {
				t.Fatalf("deterministic repair projection=%+v", scope.DeterministicRepairs)
			}
			declarationStart := strings.Index(testCase.source, "function ")
			if declarationStart < 0 {
				t.Fatal("fixture lacks function declaration")
			}
			current := testCase.source[declarationStart:]
			candidate, repaired, err := applyDirectCodingTypeScriptDeterministicRepair(current, scope)
			if err != nil {
				t.Fatal(err)
			}
			if !repaired || !strings.Contains(candidate, testCase.wantReplacement) {
				t.Fatalf("candidate=%q repaired=%t", candidate, repaired)
			}
			if err := os.WriteFile(
				filepath.Join(root, "src/fixture.ts"),
				[]byte(testCase.source[:declarationStart]+candidate), 0o600,
			); err != nil {
				t.Fatal(err)
			}
			output, err := runDirectCodingStageCommand(
				context.Background(), root, directCodingStageTimeout,
				"npm", "run", "typecheck",
			)
			if err != nil {
				t.Fatalf("deterministic repair did not compile: %v\n%s", err, output)
			}
		})
	}

	t.Run("side effecting value is not rewritten", func(t *testing.T) {
		source := `type MetricValue = null | boolean | number | string;
declare function readMetric(): MetricValue;
function ReadMetric(): number {
  const reading: number = readMetric() ?? 0;
  return reading;
}`
		if err := os.WriteFile(filepath.Join(root, "src/fixture.ts"), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
		line, column := typeScriptLexicalScopeTestLocation(t, source, "readMetric() ?? 0")
		functionOffset := strings.LastIndex(source, "function ReadMetric")
		functionLine := strings.Count(source[:functionOffset], "\n") + 1
		scope, err := inspectDirectCodingTypeScriptScope(
			context.Background(), root,
			directCodingStageDiagnostic{
				CompilerIssue: true, DocumentPath: "src/fixture.ts",
				DocumentLine: line, DocumentColumn: column,
				DocumentBlockStartLine: functionLine,
				DocumentBlockEndLine:   strings.Count(source, "\n") + 1,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(scope.DeterministicRepairs) != 0 {
			t.Fatalf("side-effecting expression received deterministic repair: %+v", scope.DeterministicRepairs)
		}
	})
}

func TestApplyTypeScriptDeterministicRepairRequiresExactByteAuthority(t *testing.T) {
	t.Parallel()
	current := "function Read(value: Value): number {\n  return value.current ?? 0;\n}"
	source := "value.current ?? 0"
	start := strings.Index(current, source)
	scope := directCodingTypeScriptScope{DeterministicRepairs: []directCodingTypeScriptDeterministicRepair{{
		Source: source, Replacement: "typeof value.current === 'number' ? value.current : 0",
		StartByte: start, EndByte: start + len(source),
	}}}
	candidate, repaired, err := applyDirectCodingTypeScriptDeterministicRepair(current, scope)
	if err != nil {
		t.Fatal(err)
	}
	if !repaired || candidate != "function Read(value: Value): number {\n  return typeof value.current === 'number' ? value.current : 0;\n}" {
		t.Fatalf("candidate=%q repaired=%t", candidate, repaired)
	}

	stale := scope
	stale.DeterministicRepairs[0].Source = "value.previous ?? 0"
	if _, _, err := applyDirectCodingTypeScriptDeterministicRepair(current, stale); err == nil ||
		!strings.Contains(err.Error(), "no longer matches") {
		t.Fatalf("stale deterministic repair error=%v", err)
	}
}

func TestTypeScriptCompilerScopeProjectionKeepsCodeSyntaxAndRejectsCompilerPaths(t *testing.T) {
	t.Parallel()
	valid := directCodingTypeScriptScope{
		Bindings: []assemblyline.TypeScriptRepairBinding{{Name: "value", Type: "string"}},
		ExpressionEvidence: []assemblyline.TypeScriptRepairExpressionEvidence{{
			Source:       `element.textContent.includes('\\d+')`,
			InferredType: "boolean",
		}},
	}
	if err := validateDirectCodingTypeScriptCompilerScopeModelProjection(valid); err != nil {
		t.Fatalf("valid TypeScript expression syntax was rejected as a path: %v", err)
	}

	pathBearingType := valid
	pathBearingType.ExpressionEvidence = []assemblyline.TypeScriptRepairExpressionEvidence{{
		Source:       "value",
		InferredType: `import("/private/staged/source").Value`,
	}}
	if err := validateDirectCodingTypeScriptCompilerScopeModelProjection(pathBearingType); err == nil {
		t.Fatal("path-bearing compiler type was accepted")
	}
}

func TestTypeScriptCompilerScopeProjectsOneExactIncompatibility(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name       string
		scope      directCodingTypeScriptScope
		wantSource string
		wantName   string
	}{
		{
			name: "numeric reading",
			scope: directCodingTypeScriptScope{
				Bindings: []assemblyline.TypeScriptRepairBinding{
					{Name: "actions", Type: "MetricActions"},
					{Name: "state", Type: "MetricState"},
				},
				UnavailableBindings: []assemblyline.TypeScriptRepairBinding{{Name: "nested", Type: "number"}},
				ExpressionEvidence: []assemblyline.TypeScriptRepairExpressionEvidence{
					{Source: "useState<number>(() => state.reading ?? 0)", InferredType: "[number, Dispatch<number>]"},
					{
						Source: "state.reading ?? 0", InferredType: "string | number | boolean",
						ContextualType: "number", IncompatibleTypes: []string{"false", "string", "true"},
						ReferencedBindings: []string{"state"},
					},
					{Source: "state.reading", InferredType: "SharedValue", ContextualType: "number", IncompatibleTypes: []string{"null", "string"}, ReferencedBindings: []string{"state"}},
				},
			},
			wantSource: "state.reading ?? 0", wantName: "state",
		},
		{
			name: "text label",
			scope: directCodingTypeScriptScope{
				Bindings: []assemblyline.TypeScriptRepairBinding{
					{Name: "entry", Type: "LabelEntry"},
					{Name: "format", Type: "(value: string) => string"},
				},
				ExpressionEvidence: []assemblyline.TypeScriptRepairExpressionEvidence{
					{Source: "entry.label ?? ''", InferredType: "number | string", ContextualType: "string", IncompatibleTypes: []string{"number"}, ReferencedBindings: []string{"entry"}},
					{Source: "entry.label", InferredType: "SharedLabel", ContextualType: "string", IncompatibleTypes: []string{"null", "number"}, ReferencedBindings: []string{"entry"}},
				},
			},
			wantSource: "entry.label ?? ''", wantName: "entry",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			projected, err := projectDirectCodingTypeScriptCompilerScope(testCase.scope)
			if err != nil {
				t.Fatal(err)
			}
			if len(projected.ExpressionEvidence) != 1 ||
				projected.ExpressionEvidence[0].Source != testCase.wantSource {
				t.Fatalf("projected expression evidence=%+v", projected.ExpressionEvidence)
			}
			if len(projected.Bindings) != 1 || projected.Bindings[0].Name != testCase.wantName {
				t.Fatalf("projected bindings=%+v", projected.Bindings)
			}
			if len(projected.UnavailableBindings) != 0 {
				t.Fatalf("projected unrelated unavailable bindings=%+v", projected.UnavailableBindings)
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

func renderTypeScriptRepairExpressionEvidenceForTest(
	evidence []assemblyline.TypeScriptRepairExpressionEvidence,
) string {
	lines := make([]string, 0, len(evidence))
	for _, item := range evidence {
		lines = append(lines, item.Source+": "+item.InferredType)
	}
	return strings.Join(lines, "\n")
}

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
			source: `// Exact regular-expression syntax remains source, not path identity.
function VerifyPattern(): void {
	  const matches = (value: string) => /\d+/.test(value);
	  void matches;
}`,
			marker: `/\d+/.test(value)`,
			want: []string{
				"VerifyPattern: () => void", "matches: (value: string) => boolean",
			},
			wantEvidence: []string{`/\d+/.test(value)`},
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

func TestApplyTypeScriptDeterministicRepairRequiresExactByteAuthority(t *testing.T) {
	t.Parallel()
	current := "function Read(value: Value): number {\n  return value.current ?? 0;\n}"
	source := "value.current ?? 0"
	start := strings.Index(current, source)
	normalizationStart := start
	scope := directCodingTypeScriptScope{DeterministicRepairs: []directCodingTypeScriptDeterministicRepair{{
		Mechanism: directCodingTypeScriptPrimitiveNullishNarrowing,
		Source:    source, Replacement: "typeof value.current === 'number' ? value.current : 0",
		StartByte: start, EndByte: start + len(source), NormalizationStartByte: &normalizationStart,
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

func TestApplyTypeScriptReferenceRepairRequiresPriorOccurrenceBytes(t *testing.T) {
	t.Parallel()
	normalization := "typeof state.value === 'string' ? state.value : ''"
	current := "function Apply(state: State): void {\n  const value = " + normalization + ";\n  record(state.value);\n}"
	normalizationStart := strings.Index(current, normalization)
	targetStart := strings.LastIndex(current, "state.value")
	repair := directCodingTypeScriptDeterministicRepair{
		Mechanism: directCodingTypeScriptPrimitiveReferenceNarrowing,
		Source:    "state.value", Replacement: normalization,
		StartByte: targetStart, EndByte: targetStart + len("state.value"),
		NormalizationStartByte: &normalizationStart,
	}
	candidate, repaired, err := applyDirectCodingTypeScriptDeterministicRepair(
		current, directCodingTypeScriptScope{DeterministicRepairs: []directCodingTypeScriptDeterministicRepair{repair}},
	)
	if err != nil || !repaired || strings.Count(candidate, normalization) != 2 {
		t.Fatalf("exact occurrence candidate=%q repaired=%t error=%v", candidate, repaired, err)
	}
	outOfAuthority := int(^uint(0) >> 1)
	repair.NormalizationStartByte = &outOfAuthority
	if _, _, err := applyDirectCodingTypeScriptDeterministicRepair(
		current, directCodingTypeScriptScope{DeterministicRepairs: []directCodingTypeScriptDeterministicRepair{repair}},
	); err == nil || !strings.Contains(err.Error(), "outside its prior authority") {
		t.Fatalf("out-of-authority normalization error=%v", err)
	}
}

func TestTypeScriptScopeInspectorRequiresExactCodeOwnedSource(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := writeDirectCodingTypeScriptScopeInspector(root); err != nil {
		t.Fatal(err)
	}
	if err := validateDirectCodingTypeScriptScopeInspector(root); err != nil {
		t.Fatalf("fresh inspector rejected: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(root, directCodingTypeScriptScopeInspectorFile),
		[]byte("process.stdout.write('{}');"), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := validateDirectCodingTypeScriptScopeInspector(root); err == nil ||
		!strings.Contains(err.Error(), "differs from its code-owned source") {
		t.Fatalf("mutated inspector error=%v", err)
	}
}

func TestTypeScriptCompilerScopeProjectionKeepsCodeSyntaxAndRejectsCompilerPaths(t *testing.T) {
	t.Parallel()
	valid := directCodingTypeScriptScope{
		Bindings: []assemblyline.TypeScriptRepairBinding{
			{Name: "value", Type: "string"},
			{
				Name: "handleInput", Type: "(e: React.ChangeEvent<HTMLInputElement>) => void",
				CallableSignatures: []string{"(e: React.ChangeEvent<HTMLInputElement>) => void"},
			},
			{
				Name: "handlePointer", Type: "(x: PointerEvent) => boolean",
				CallableSignatures: []string{"(x: PointerEvent) => boolean"},
			},
		},
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

	pathBearingBinding := valid
	pathBearingBinding.Bindings = []assemblyline.TypeScriptRepairBinding{{
		Name: "load", Type: `typeof import("/private/staged/source").load`,
	}}
	if err := validateDirectCodingTypeScriptCompilerScopeModelProjection(pathBearingBinding); err == nil {
		t.Fatal("path-bearing compiler binding type was accepted")
	}
}

func TestTypeScriptCompilerScopeProjectsBeforeModelVisibleValidation(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name          string
		referenced    assemblyline.TypeScriptRepairBinding
		unrelatedPath assemblyline.TypeScriptRepairBinding
		evidence      assemblyline.TypeScriptRepairExpressionEvidence
	}{
		{
			name:          "telemetry reading",
			referenced:    assemblyline.TypeScriptRepairBinding{Name: "metric", Type: "MetricState"},
			unrelatedPath: assemblyline.TypeScriptRepairBinding{Name: "loadArchive", Type: `typeof import("/private/staged/archive").loadArchive`},
			evidence: assemblyline.TypeScriptRepairExpressionEvidence{
				Source: "metric.reading ?? 0", InferredType: "number | string",
				ContextualType: "number", IncompatibleTypes: []string{"string"},
				ReferencedBindings: []string{"metric"},
			},
		},
		{
			name:          "catalog label",
			referenced:    assemblyline.TypeScriptRepairBinding{Name: "entry", Type: "CatalogEntry"},
			unrelatedPath: assemblyline.TypeScriptRepairBinding{Name: "hydrate", Type: `typeof import("C:\\private\\catalog").hydrate`},
			evidence: assemblyline.TypeScriptRepairExpressionEvidence{
				Source: "entry.label ?? ''", InferredType: "number | string",
				ContextualType: "string", IncompatibleTypes: []string{"number"},
				ReferencedBindings: []string{"entry"},
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			projected, err := projectDirectCodingTypeScriptCompilerScopeForModel(
				directCodingTypeScriptScope{
					Bindings:           []assemblyline.TypeScriptRepairBinding{testCase.referenced, testCase.unrelatedPath},
					ExpressionEvidence: []assemblyline.TypeScriptRepairExpressionEvidence{testCase.evidence},
				},
			)
			if err != nil {
				t.Fatalf("unrelated compiler path was validated before projection: %v", err)
			}
			if len(projected.Bindings) != 1 || projected.Bindings[0].Name != testCase.referenced.Name {
				t.Fatalf("projected bindings=%+v", projected.Bindings)
			}
		})
	}

	selectedPath := directCodingTypeScriptScope{
		Bindings: []assemblyline.TypeScriptRepairBinding{{
			Name: "entry", Type: `import("/private/staged/catalog").Entry`,
		}},
		ExpressionEvidence: []assemblyline.TypeScriptRepairExpressionEvidence{{
			Source: "entry.label", InferredType: "number | string", ContextualType: "string",
			IncompatibleTypes: []string{"number"}, ReferencedBindings: []string{"entry"},
		}},
	}
	if _, err := projectDirectCodingTypeScriptCompilerScopeForModel(selectedPath); err == nil {
		t.Fatal("path-bearing selected compiler binding was accepted")
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
	files, err := typeScriptBrowserStaticFiles(
		requireDirectCodingVersionProfile(t, typeScriptBrowserVersionProfileV1),
		"scope-fixture", "Scope fixture", "",
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if file.Path != "package.json" && file.Path != "package-lock.json" && file.Path != "tsconfig.json" {
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

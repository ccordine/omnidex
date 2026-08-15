package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTypeScriptCompilerFeedbackPreservesOneExactDeclarationLocation(t *testing.T) {
	t.Parallel()
	longType := "'" + strings.Repeat("bounded_compiler_type_", 19) + "'"
	for _, fixture := range []struct {
		name       string
		path       string
		signature  string
		source     string
		location   func(string, int, int, string) string
		message    string
		other      string
		wantLine   int
		wantColumn int
	}{
		{
			name: "TypeScript arithmetic", path: "src/totals.ts",
			signature: "function SumTotals(value: string): number",
			source:    "function SumTotals(value: string): number {\n  const normalized = value.trim();\n  return normalized + 1;\n}",
			location: func(path string, line int, column int, message string) string {
				return fmt.Sprintf("%s(%d,%d): %s", path, line, column, message)
			},
			message:  "error TS2362: The left-hand side of an arithmetic operation must be of type any, number, bigint or an enum type " + longType + ".",
			other:    "error TS2304: Cannot find name 'unrelatedTotal'.",
			wantLine: 3, wantColumn: 10,
		},
		{
			name: "TSX rendered value", path: "src/panel.tsx",
			signature: "function InventoryPanel(value: SharedValue): ReactElement",
			source:    "function InventoryPanel(value: SharedValue): ReactElement {\n  const label = value;\n  return <output>{label}</output>;\n}",
			location: func(path string, line int, column int, message string) string {
				return fmt.Sprintf("%s:%d:%d %s", path, line, column, message)
			},
			message:  "error TS2322: Type 'SharedValue' is not assignable to type 'ReactNode'.",
			other:    "error TS2552: Cannot find name 'differentPanel'.",
			wantLine: 3, wantColumn: 19,
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			document := assemblyline.TypeScriptDocument{
				ID: "fixture", Path: fixture.path, Header: "type SharedValue = null | boolean | number | string;",
				Blocks: []assemblyline.TypeScriptBlock{{
					ID: "fixture.block", Signature: fixture.signature,
					Contract: "Return one derived value.", API: fixture.signature,
				}},
			}
			composed, err := assemblyline.ComposeTypeScriptDocument(document, map[string]string{
				"fixture.block": fixture.source,
			})
			if err != nil {
				t.Fatal(err)
			}
			span := composed.Spans["fixture.block"]
			output := strings.Join([]string{
				fixture.location(fixture.path, span.StartLine+fixture.wantLine-1, fixture.wantColumn, fixture.message),
				fixture.location(fixture.path, span.StartLine+4, 7, fixture.other),
			}, "\n")
			diagnostic, mapped := mapDirectCodingTypeScriptStageDiagnostic(
				[]assemblyline.ComposedTypeScriptDocument{composed}, output,
			)
			if !mapped {
				t.Fatal("compiler diagnostic was not mapped")
			}
			want := fmt.Sprintf(
				"DECLARATION_LOCATION: line %d column %d\nTYPESCRIPT_DIAGNOSTIC: %s",
				fixture.wantLine, fixture.wantColumn, fixture.message,
			)
			if diagnostic.ModelFeedback != want {
				t.Fatalf("model feedback=%q want %q", diagnostic.ModelFeedback, want)
			}
			if !diagnostic.CompilerIssue || diagnostic.DocumentPath != fixture.path ||
				diagnostic.DocumentLine != span.StartLine+fixture.wantLine-1 ||
				diagnostic.DocumentColumn != fixture.wantColumn ||
				diagnostic.DocumentBlockStartLine != span.StartLine ||
				diagnostic.DocumentBlockEndLine != span.EndLine {
				t.Fatalf("compiler location authority=%+v span=%+v", diagnostic, span)
			}
			if strings.Contains(diagnostic.ModelFeedback, fixture.path) ||
				strings.Contains(diagnostic.ModelFeedback, fixture.other) {
				t.Fatalf("feedback leaked path or neighbor diagnostic: %q", diagnostic.ModelFeedback)
			}
			if len(want) > 360 && len(diagnostic.ModelFeedback) != len(want) {
				t.Fatalf("compiler feedback was arbitrarily truncated: got=%d want=%d", len(diagnostic.ModelFeedback), len(want))
			}
		})
	}
}

func TestTypeScriptCompilerScopeReceiptRejectsInexactOrMissingAuthority(t *testing.T) {
	t.Parallel()
	valid := `{"schema":"omnidex.typescript-lexical-scope.v1","bindings":[{"name":"value","type":"number"}],"unavailable_bindings":[]}`
	for name, raw := range map[string]string{
		"duplicate field":      `{"schema":"omnidex.typescript-lexical-scope.v1","schema":"omnidex.typescript-lexical-scope.v1","bindings":[{"name":"value","type":"number"}],"unavailable_bindings":[]}`,
		"unknown field":        `{"schema":"omnidex.typescript-lexical-scope.v1","bindings":[{"name":"value","type":"number","path":"private.ts"}],"unavailable_bindings":[]}`,
		"missing bindings":     `{"schema":"omnidex.typescript-lexical-scope.v1"}`,
		"missing unavailable":  `{"schema":"omnidex.typescript-lexical-scope.v1","bindings":[{"name":"value","type":"number"}]}`,
		"empty bindings":       `{"schema":"omnidex.typescript-lexical-scope.v1","bindings":[],"unavailable_bindings":[]}`,
		"unsorted bindings":    `{"schema":"omnidex.typescript-lexical-scope.v1","bindings":[{"name":"z","type":"number"},{"name":"a","type":"number"}],"unavailable_bindings":[]}`,
		"overlapping bindings": `{"schema":"omnidex.typescript-lexical-scope.v1","bindings":[{"name":"value","type":"number"}],"unavailable_bindings":[{"name":"value","type":"number"}]}`,
		"trailing value":       valid + `{}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeDirectCodingTypeScriptScopeReceipt([]byte(raw)); err == nil {
				t.Fatalf("accepted invalid scope receipt: %s", raw)
			}
		})
	}
}

func TestTypeScriptCompilerCorrectionCannotUseTestSummaryTruncation(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"v3_coding_typescript_compiler_feedback.go",
		"v3_coding_typescript_stage_workspace.go",
		"exact_station_convergence_compiler.go",
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		source := string(raw)
		for _, forbidden := range []string{
			"directCodingTypeScriptTestModelFailure", "maxDirectCodingTestFailureLines",
		} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("compiler correction %s uses test-only summary gate %q", path, forbidden)
			}
		}
	}
}

func TestTypeScriptCompilerFeedbackPreservesPathFreeTypeSemantics(t *testing.T) {
	t.Parallel()
	for _, message := range []string{
		"error TS7053: Element implicitly has an 'any' type because expression of type 'number' can't be used to index type 'SharedValue'. No index signature with a parameter of type 'number' was found on type 'SharedValue'.",
		"error TS2339: Property 'visible.label' does not exist on type 'InventoryRecord'.",
		"error TS2488: Type 'SharedValue' must have a '[Symbol.iterator]()' method that returns an iterator.",
	} {
		feedback := directCodingTypeScriptLocatedCompilerFailure(7, 11, message)
		want := "DECLARATION_LOCATION: line 7 column 11\nTYPESCRIPT_DIAGNOSTIC: " + message
		if feedback != want {
			t.Fatalf("path-free compiler semantics were discarded:\nGOT:  %q\nWANT: %q", feedback, want)
		}
	}
}

func TestTypeScriptCompilerFeedbackRejectsRemainingFileIdentity(t *testing.T) {
	t.Parallel()
	for _, message := range []string{
		"error TS2307: Cannot find module '../private/generated' or its corresponding type declarations.",
		"error TS6053: File 'private-config.json' not found.",
	} {
		if feedback := directCodingTypeScriptLocatedCompilerFailure(3, 5, message); feedback != "" {
			t.Fatalf("compiler feedback retained file identity: %q", feedback)
		}
	}
}

func TestTypeScriptCompilerCorrectionReceivesAndReplacesOnlyASTOwner(t *testing.T) {
	t.Parallel()
	current := `function Toggle(index: number, actions: Actions): void {
  const [muteList, setMuteList] = useState<boolean[]>([]);
  const handleMuteToggle = useCallback((selected: number) => {
    setMuteList(previous => {
      const nextMuted = !previous[selected];
      const next = [...previous];
      next[selected] = nextMuted;
      return next;
    });
    actions.set(selected, nextMuted);
  }, [actions]);
  const untouched = index;
  handleMuteToggle(untouched);
}`
	fixedRegion := `  const handleMuteToggle = useCallback((selected: number) => {
    setMuteList(previous => {
      const nextMuted = !previous[selected];
      const next = [...previous];
      next[selected] = nextMuted;
      actions.set(selected, nextMuted);
      return next;
    });
  }, [actions]);`
	line, column := typeScriptWorkerSourceLocation(t, current, "nextMuted);")
	region, err := assemblyline.NewTypeScriptCompilerRepairRegion(
		current, false, line, column,
		[]assemblyline.TypeScriptRepairBinding{{Name: "value", Type: "unknown"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	models := make([]string, 0, 2)
	const instruction = "Move actions.set(selected, nextMuted) into the setMuteList callback immediately before return next, where nextMuted is available; preserve every other statement."
	block := assemblyline.TypeScriptBlock{
		ID: "toggle.handler", Signature: "function Toggle(index: number, actions: Actions): void",
		API: "function Toggle(index: number, actions: Actions): void", Globals: []string{"useCallback", "useState"},
	}
	const available = "interface Actions { set(index: number, value: boolean): void }"
	const failure = "DECLARATION_LOCATION: line 10 column 27\nTYPESCRIPT_DIAGNOSTIC: error TS2304: Cannot find name 'nextMuted'."
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1, CorrectionModel: "executor",
		Execute: func(portable assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
			calls++
			models = append(models, model)
			prompt, _, err := assemblyline.RenderPortableJob(portable)
			if err != nil {
				return assemblyline.PortableResult{}, err
			}
			switch portable.Kind {
			case assemblyline.WorkTypeScriptRepairGuidance:
				var analysis assemblyline.TypeScriptRepairGuidanceInput
				if err := json.Unmarshal(portable.Payload, &analysis); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if analysis.CurrentDeclaration != "" || analysis.RepairRegion == nil ||
					analysis.RepairRegion.Source != region.Source || analysis.Diagnostic != failure {
					t.Fatalf("repair analyst did not receive exact diagnostic authority: %+v", analysis)
				}
				if len(analysis.Capabilities) != 1 || analysis.Capabilities[0] != available ||
					!strings.Contains(strings.Join(analysis.PermittedSymbols, ","), "useCallback") {
					t.Fatalf("repair analyst lost declarations or symbols: %+v", analysis)
				}
				return assemblyline.PortableResult{
					JobID: portable.ID, Candidate: `{"instruction":"` + instruction + `"}`,
				}, nil
			case assemblyline.WorkFragmentCorrection:
				var correction assemblyline.FragmentCorrectionInput
				if err := json.Unmarshal(portable.Payload, &correction); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if correction.CurrentDeclaration != "" || correction.RepairRegion == nil ||
					correction.RepairRegion.Source != region.Source ||
					correction.RepairGuidance != instruction || correction.Diagnostic != "" ||
					correction.RequiredChange != "" || len(correction.Capabilities) != 0 ||
					len(correction.PermittedSymbols) != 0 {
					t.Fatalf("repair executor retained analyst responsibility: %+v", correction)
				}
				for _, forbidden := range []string{
					"TYPESCRIPT_DIAGNOSTIC", "Cannot find name", "interface Actions",
					"LOCAL_BINDINGS", "ALREADY_IN_SCOPE_IDENTIFIERS", "const untouched",
				} {
					if strings.Contains(prompt, forbidden) {
						t.Fatalf("repair executor prompt retained %q:\n%s", forbidden, prompt)
					}
				}
				if !strings.Contains(prompt, instruction) ||
					!strings.Contains(prompt, "actions.set(selected, nextMuted)") {
					t.Fatalf("repair executor did not receive only guidance and owner source:\n%s", prompt)
				}
				return assemblyline.PortableResult{JobID: portable.ID, Candidate: fixedRegion}, nil
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected repair work kind %s", portable.Kind)
			}
		},
	}
	guidance, err := runDirectCodingTypeScriptRepairGuidance(
		runtime, "analyst", block, available, current, &region, failure,
	)
	if err != nil {
		t.Fatal(err)
	}
	source, err := runDirectCodingTypeScriptFragmentWorker(runtime, "executor", directCodingTypeScriptFragmentJob{
		block: block, available: available, current: current, repairRegion: &region,
		repairGuidance: guidance,
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || strings.Join(models, ",") != "analyst,executor" ||
		!strings.Contains(source, "actions.set(selected, nextMuted);\n      return next;") ||
		!strings.Contains(source, "const untouched = index;") {
		t.Fatalf("calls=%d models=%v spliced source:\n%s", calls, models, source)
	}
}

func typeScriptWorkerSourceLocation(t *testing.T, source string, marker string) (int, int) {
	t.Helper()
	offset := strings.Index(source, marker)
	if offset < 0 {
		t.Fatalf("source lacks marker %q", marker)
	}
	prefix := source[:offset]
	return strings.Count(prefix, "\n") + 1, offset - strings.LastIndex(prefix, "\n")
}

func TestTypeScriptRepairAnalystRetainsCanonicalCompilerFeedbackBytes(t *testing.T) {
	t.Parallel()
	failure := "DECLARATION_LOCATION: line 17 column 9\nTYPESCRIPT_DIAGNOSTIC: error TS2322: " +
		strings.TrimSpace(strings.Repeat("Type 'RecordedValue' is not assignable to 'VisibleValue'. ", 7))
	job, err := assemblyline.NewTypeScriptRepairGuidanceJob(assemblyline.TypeScriptRepairGuidanceInput{
		Language: "typescript", Signature: "function RecordsView(): ReactElement",
		CurrentDeclaration: "function RecordsView(): ReactElement { return <div />; }",
		Diagnostic:         failure,
	})
	if err != nil {
		t.Fatal(err)
	}
	var analysis assemblyline.TypeScriptRepairGuidanceInput
	if err := json.Unmarshal(job.Payload, &analysis); err != nil {
		t.Fatal(err)
	}
	if analysis.Diagnostic != failure {
		t.Fatalf("canonical compiler feedback changed:\nGOT:  %q\nWANT: %q", analysis.Diagnostic, failure)
	}
}

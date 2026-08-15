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
	region, err := assemblyline.NewTypeScriptCompilerRepairRegion(current, false, line, column)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), CorrectionModel: "qwen2.5-coder:7b",
		Execute: func(portable assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			calls++
			var correction assemblyline.FragmentCorrectionInput
			if err := json.Unmarshal(portable.Payload, &correction); err != nil {
				return assemblyline.PortableResult{}, err
			}
			if correction.CurrentDeclaration != "" || correction.RepairRegion == nil ||
				correction.RepairRegion.Source != region.Source {
				t.Fatalf("correction did not isolate compiler owner: %+v", correction)
			}
			if len(correction.Capabilities) != 1 || correction.Capabilities[0] != "interface Actions { set(index: number, value: boolean): void }" ||
				!strings.Contains(strings.Join(correction.PermittedSymbols, ","), "useCallback") {
				t.Fatalf("localized correction lost required declarations or symbols: %+v", correction)
			}
			prompt, _, err := assemblyline.RenderPortableJob(portable)
			if err != nil {
				return assemblyline.PortableResult{}, err
			}
			if strings.Contains(prompt, "const untouched") || !strings.Contains(prompt, "actions.set(selected, nextMuted)") {
				t.Fatalf("prompt did not expose only the exact failing owner:\n%s", prompt)
			}
			return assemblyline.PortableResult{JobID: portable.ID, Candidate: fixedRegion}, nil
		},
	}
	source, err := runDirectCodingTypeScriptFragmentWorker(runtime, "qwen2.5-coder:7b", directCodingTypeScriptFragmentJob{
		block: assemblyline.TypeScriptBlock{
			ID: "toggle.handler", Signature: "function Toggle(index: number, actions: Actions): void",
			API: "function Toggle(index: number, actions: Actions): void", Globals: []string{"useCallback", "useState"},
		},
		available: "interface Actions { set(index: number, value: boolean): void }",
		current:   current, repairRegion: &region,
		failure: "DECLARATION_LOCATION: line 10 column 27\nTYPESCRIPT_DIAGNOSTIC: error TS2304: Cannot find name 'nextMuted'.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !strings.Contains(source, "actions.set(selected, nextMuted);\n      return next;") ||
		!strings.Contains(source, "const untouched = index;") {
		t.Fatalf("calls=%d spliced source:\n%s", calls, source)
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

func TestTypeScriptCorrectionRetainsCanonicalCompilerFeedbackBytes(t *testing.T) {
	t.Parallel()
	failure := "DECLARATION_LOCATION: line 17 column 9\nTYPESCRIPT_DIAGNOSTIC: error TS2322: " +
		strings.TrimSpace(strings.Repeat("Type 'RecordedValue' is not assignable to 'VisibleValue'. ", 7))
	job, err := newDirectCodingTypeScriptPortableJob(directCodingTypeScriptFragmentJob{
		block: assemblyline.TypeScriptBlock{
			ID: "records.view", Signature: "function RecordsView(): ReactElement",
			Contract: "Render records.", API: "function RecordsView(): ReactElement",
		},
		tsx: true, current: "function RecordsView(): ReactElement { return <div />; }",
		failure: failure,
	})
	if err != nil {
		t.Fatal(err)
	}
	var correction assemblyline.FragmentCorrectionInput
	if err := decodeReplayCorrectionInput(job, &correction); err != nil {
		t.Fatal(err)
	}
	if correction.Diagnostic != failure {
		t.Fatalf("canonical compiler feedback changed:\nGOT:  %q\nWANT: %q", correction.Diagnostic, failure)
	}
}

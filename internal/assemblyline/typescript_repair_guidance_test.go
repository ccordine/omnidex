package assemblyline

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/modelcontext"
)

func TestTypeScriptRepairGuidanceAndExecutionHaveDisjointAuthority(t *testing.T) {
	t.Parallel()
	region := TypeScriptFragmentRepairRegion{
		Kind: TypeScriptRepairRegionCompilerOwner, StartLine: 4, EndLine: 6,
		Source: "  values.forEach(value => {\n    actions.set(index, value);\n  });",
		Bindings: []TypeScriptRepairBinding{
			{Name: "actions", Type: "Actions", Members: []string{"set: (index: number, value: number) => void"}},
			{Name: "index", Type: "number"},
			{Name: "values", Type: "number[]"},
		},
		UnavailableBindings: []TypeScriptRepairBinding{{Name: "value", Type: "number"}},
	}
	analysis, err := NewTypeScriptRepairGuidanceJob(TypeScriptRepairGuidanceInput{
		Language: "typescript", Dialect: "TypeScript function syntax",
		Signature: "function Apply(index: number, actions: Actions): void",
		Capabilities: []string{
			"interface Actions { set(index: number, value: number): void }",
		},
		PermittedSymbols: []string{"useMemo"}, RepairRegion: &region,
		Diagnostic: "DECLARATION_LOCATION: line 7 column 24\nTYPESCRIPT_DIAGNOSTIC: error TS2304: Cannot find name 'value'.",
	})
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(analysis)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"one self-contained imperative source-transformation instruction", "Resolve only the observed failure",
		"EXACT_VALIDATION_FAILURE:",
		"Cannot find name 'value'", "BINDINGS_AVAILABLE_AT_FAILURE_JSON:",
		"NESTED_BINDINGS_UNAVAILABLE_AT_FAILURE_JSON:", "interface Actions",
		"BINDINGS_AVAILABLE_AT_FAILURE_JSON, language-predeclared identifiers",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("analysis prompt omitted %q:\n%s", required, prompt)
		}
	}
	for _, forbidden := range []string{"repair analyst", "separate source executor"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("analysis prompt retained agent-role language %q:\n%s", forbidden, prompt)
		}
	}
	if strings.Contains(prompt, "The two external-authority lists below are exhaustive.") {
		t.Fatalf("region guidance contradicted compiler-proven bindings:\n%s", prompt)
	}

	const instruction = "Move actions.set(index, value) into the values.forEach callback where value is in lexical scope; preserve the callback and all other statements."
	guidance, err := DecodeTypeScriptRepairGuidanceResult(analysis, instruction)
	if err != nil || guidance.Instruction != instruction {
		t.Fatalf("guidance=%+v err=%v", guidance, err)
	}
	if _, err := DecodeTypeScriptRepairGuidanceResult(analysis, `{"instruction":"change it"}`); err == nil {
		t.Fatal("accepted JSON wrapper")
	}
	execution, err := NewFragmentCorrectionJob(FragmentCorrectionInput{
		Language: "typescript", Signature: "function Apply(index: number, actions: Actions): void",
		RepairRegion: &region, RepairGuidance: instruction,
	})
	if err != nil {
		t.Fatal(err)
	}
	executionPrompt, err := RenderPortableJob(execution)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"EXACT_MUTABLE_SOURCE_JSON:", "REQUIRED_SOURCE_TRANSFORMATION:", instruction} {
		if !strings.Contains(executionPrompt, required) {
			t.Fatalf("execution prompt omitted %q:\n%s", required, executionPrompt)
		}
	}
	for _, forbidden := range []string{
		"Cannot find name", "TYPESCRIPT_DIAGNOSTIC", "interface Actions", "useMemo",
		"BINDINGS_AVAILABLE", "value\"", "REQUIRED_DECLARATION_SIGNATURE",
	} {
		if strings.Contains(executionPrompt, forbidden) {
			t.Fatalf("execution prompt retained analyst authority %q:\n%s", forbidden, executionPrompt)
		}
	}
	var wire map[string]any
	if err := json.Unmarshal(execution.Payload, &wire); err != nil {
		t.Fatal(err)
	}
	if _, exists := wire["diagnostic"]; exists {
		t.Fatalf("repair execution payload retained raw diagnostic: %s", execution.Payload)
	}
	if _, exists := wire["required_change"]; exists {
		t.Fatalf("repair execution payload retained mixed code-owned advice: %s", execution.Payload)
	}
}

func TestTypeScriptRepairGuidanceUsesTypedSourcePathBoundary(t *testing.T) {
	t.Parallel()
	input := TypeScriptRepairGuidanceInput{
		Language: "typescript", Dialect: "TypeScript function syntax",
		Signature: "function Ratio(left: number, right: number): boolean",
		CurrentDeclaration: `function Ratio(left: number, right: number): boolean {
  const fraction = left / right;
  return /\d+\/\d+/.test(String(fraction));
}`,
		Diagnostic: "error TS2322: The observed result has the wrong type.",
	}
	if _, err := NewTypeScriptRepairGuidanceJob(input); err != nil {
		t.Fatalf("parser-proven division or regex was rejected: %v", err)
	}
	input.CurrentDeclaration = `function Ratio(): boolean { return "../private/value"; }`
	if _, err := NewTypeScriptRepairGuidanceJob(input); err == nil {
		t.Fatal("path-bearing source literal was accepted")
	}
}

func TestTypeScriptRepairGuidanceRejectsKnownBareArtifactWithProvenance(t *testing.T) {
	t.Parallel()
	provenance, err := modelcontext.NewArtifactIdentityProvenance([]string{"internal/transport.go"})
	if err != nil {
		t.Fatal(err)
	}
	guidance := TypeScriptRepairGuidance{Instruction: "Move the value into transport.go."}
	if err := guidance.Validate(); err != nil {
		t.Fatalf("bare dotted atom was inferred without provenance: %v", err)
	}
	if err := guidance.ValidatePathFree(provenance); err == nil {
		t.Fatal("known bare artifact survived provenance-aware guidance acceptance")
	}
}

func TestTypeScriptRepairGuidanceResultHasNoRawRegexPathException(t *testing.T) {
	t.Parallel()
	job, err := NewTypeScriptRepairGuidanceJob(TypeScriptRepairGuidanceInput{
		Language: "typescript", Dialect: "TypeScript function syntax",
		Signature:          "function Inventory(): string",
		CurrentDeclaration: "function Inventory(): string { return 'available'; }",
		Diagnostic: "Unable to find the regular expression pattern formed from ordered components " +
			`[source text "out"; one hyphen-minus character (U+002D); source text "of"; one hyphen-minus character (U+002D); source text "stock"]. Active flags [source flag "i": case-insensitive matching].`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeTypeScriptRepairGuidanceResult(
		job, "Change the label to match /out-of-stock/i.",
	); err == nil || !strings.Contains(err.Error(), "filesystem identity") {
		t.Fatalf("raw regex-shaped guidance bypassed strict result validation: %v", err)
	}
}

func TestTypeScriptRepairGuidanceRequiresExactSourceAndFailure(t *testing.T) {
	t.Parallel()
	base := TypeScriptRepairGuidanceInput{
		Language: "typescript", Dialect: "TypeScript function syntax",
		Signature:          "function Apply(): void",
		CurrentDeclaration: "function Apply(): void { run(); }",
		Diagnostic:         "error TS2304: Cannot find name 'run'.",
	}
	for name, mutate := range map[string]func(*TypeScriptRepairGuidanceInput){
		"missing source": func(input *TypeScriptRepairGuidanceInput) {
			input.CurrentDeclaration = ""
		},
		"two sources": func(input *TypeScriptRepairGuidanceInput) {
			input.RepairRegion = &TypeScriptFragmentRepairRegion{
				Kind: TypeScriptRepairRegionCompilerOwner, StartLine: 1, EndLine: 1,
				Source: "run();", Bindings: []TypeScriptRepairBinding{{Name: "run", Type: "() => void"}},
			}
		},
		"missing failure": func(input *TypeScriptRepairGuidanceInput) {
			input.Diagnostic = ""
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := base
			mutate(&input)
			if _, err := NewTypeScriptRepairGuidanceJob(input); err == nil {
				t.Fatalf("accepted invalid guidance input: %#v", input)
			}
		})
	}
}

func TestGuidedFragmentCorrectionRejectsMixedDiagnosticAuthority(t *testing.T) {
	t.Parallel()
	_, err := NewFragmentCorrectionJob(FragmentCorrectionInput{
		Language: "typescript", Signature: "function Apply(): void",
		CurrentDeclaration: "function Apply(): void { run(); }",
		RepairGuidance:     "Call run exactly once.",
		RequiredChange:     "Fix it.", Diagnostic: "run failed.",
	})
	if err == nil || !strings.Contains(err.Error(), "raw diagnostic") {
		t.Fatalf("mixed repair authority error=%v", err)
	}
}

package assemblyline

import (
	"encoding/json"
	"strings"
	"testing"
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
		Language:  "typescript",
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
	prompt, schema, err := RenderPortableJob(analysis)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"repair analyst", "separate source executor", "EXACT_VALIDATION_FAILURE:",
		"Cannot find name 'value'", "BINDINGS_AVAILABLE_AT_FAILURE_JSON:",
		"NESTED_BINDINGS_UNAVAILABLE_AT_FAILURE_JSON:", "interface Actions",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("analysis prompt omitted %q:\n%s", required, prompt)
		}
	}
	if schema == nil || schema["additionalProperties"] != false {
		t.Fatalf("repair guidance schema is not closed: %#v", schema)
	}

	const instruction = "Move actions.set(index, value) into the values.forEach callback where value is in lexical scope; preserve the callback and all other statements."
	execution, err := NewFragmentCorrectionJob(FragmentCorrectionInput{
		Language: "typescript", Signature: "function Apply(index: number, actions: Actions): void",
		RepairRegion: &region, RepairGuidance: instruction,
	})
	if err != nil {
		t.Fatal(err)
	}
	executionPrompt, executionSchema, err := RenderPortableJob(execution)
	if err != nil {
		t.Fatal(err)
	}
	if executionSchema != nil {
		t.Fatalf("repair execution unexpectedly has a structured schema: %#v", executionSchema)
	}
	for _, required := range []string{"EXACT_MUTABLE_SOURCE_JSON:", "REPAIR_INSTRUCTION:", instruction} {
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

func TestTypeScriptRepairGuidanceRequiresExactSourceAndFailure(t *testing.T) {
	t.Parallel()
	base := TypeScriptRepairGuidanceInput{
		Language: "typescript", Signature: "function Apply(): void",
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
	if err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("mixed repair authority error=%v", err)
	}
}

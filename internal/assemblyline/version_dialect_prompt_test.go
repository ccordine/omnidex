package assemblyline

import (
	"strings"
	"testing"
)

func TestGenerationAndRepairAnalysisCarryDialectWhileRepairExecutionDoesNot(t *testing.T) {
	const dialect = "registered source dialect sentinel"
	initialPrompts := make([]string, 0, 4)
	goPrompt, err := BuildGoFragmentGenerationPrompt(FragmentGenerationInput{
		Language: "go", Dialect: dialect, Signature: "func Value() int", Behavior: "Return one.",
	})
	if err != nil {
		t.Fatal(err)
	}
	initialPrompts = append(initialPrompts, goPrompt)
	goModificationPrompt, err := BuildGoFragmentModificationPrompt(FragmentModificationInput{
		Language: "go", Dialect: dialect, Signature: "func Value() int",
		CurrentDeclaration: "func Value() int { return 0 }",
		RequirementQuote:   "Return one.",
	})
	if err != nil {
		t.Fatal(err)
	}
	initialPrompts = append(initialPrompts, goModificationPrompt)
	boundedPrompt, err := BuildBoundedSourceFragmentGenerationPrompt(FragmentGenerationInput{
		Language: "javascript", Dialect: dialect,
		Signature: "function value()", Behavior: "Return one.",
	})
	if err != nil {
		t.Fatal(err)
	}
	initialPrompts = append(initialPrompts, boundedPrompt)
	typeScriptPrompt, err := BuildTypeScriptFragmentPrompt(TypeScriptFragmentPrompt{
		Dialect: dialect, Signature: "function Value(): number", Contract: "Return one.",
	})
	if err != nil {
		t.Fatal(err)
	}
	initialPrompts = append(initialPrompts, typeScriptPrompt)
	for index, prompt := range initialPrompts {
		if !strings.Contains(prompt, "SOURCE_DIALECT:\n"+dialect) {
			t.Fatalf("initial prompt %d omits source dialect:\n%s", index, prompt)
		}
	}

	repairExecution, err := BuildTypeScriptFragmentPrompt(TypeScriptFragmentPrompt{
		Signature:      "function Value(): number",
		Current:        "function Value(): number { return 0; }",
		RepairGuidance: "Replace the numeric literal zero with one.",
	})
	if err != nil {
		t.Fatal(err)
	}
	repairGuidance, err := BuildTypeScriptRepairGuidancePrompt(TypeScriptRepairGuidanceInput{
		Language: "typescript", Dialect: dialect, Signature: "function Value(): number",
		CurrentDeclaration: "function Value(): number { return 0; }",
		Diagnostic:         "error TS2322: The returned value has the wrong type.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(repairExecution, "SOURCE_DIALECT") || strings.Contains(repairExecution, dialect) {
		t.Fatalf("repair executor received diagnostic-analysis dialect authority:\n%s", repairExecution)
	}
	if !strings.Contains(repairGuidance, "SOURCE_DIALECT:\n"+dialect) {
		t.Fatalf("repair guidance omitted exact source dialect:\n%s", repairGuidance)
	}
}

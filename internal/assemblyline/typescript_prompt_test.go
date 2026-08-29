package assemblyline

import (
	"strings"
	"testing"
)

func TestTypeScriptFragmentPromptContainsOnlyLocalAPIContext(t *testing.T) {
	prompt, err := BuildTypeScriptFragmentPrompt(TypeScriptFragmentPrompt{
		Dialect:   "TypeScript 5.9.3 with TSX",
		Signature: "function double(value: Value): Value",
		Contract:  "Return a new value whose amount is twice the supplied amount.",
		Available: "interface Value { amount: number }",
		Globals:   []string{"Math"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"workspace", "filename", "path", "project tree", "worker", "agent", "dependency graph", "job"} {
		if strings.Contains(strings.ToLower(prompt), forbidden) {
			t.Fatalf("prompt leaked %q:\n%s", forbidden, prompt)
		}
	}
	if !strings.Contains(prompt, "interface Value") || !strings.Contains(prompt, "raw code only") {
		t.Fatalf("prompt=%s", prompt)
	}
}

func TestTypeScriptFragmentPromptRejectsBloatedInitialEnvelope(t *testing.T) {
	_, err := BuildTypeScriptFragmentPrompt(TypeScriptFragmentPrompt{
		Dialect:   "TypeScript 5.9.3 with TSX",
		Signature: "function " + strings.Repeat("a", maxTypeScriptInitialEnvelopeBytes) + "(): void",
		Contract:  "Do the one supplied operation.",
	})
	if err == nil || !strings.Contains(err.Error(), "initial envelope") {
		t.Fatalf("error=%v", err)
	}
}

func TestGuidedTypeScriptCorrectionPromptHasOnlyInstructionAndMutableSource(t *testing.T) {
	t.Parallel()
	current := "function render(): ReactElement { return <div />; }"
	instruction := "Replace the returned div with a button element."
	prompt, err := BuildTypeScriptFragmentPrompt(TypeScriptFragmentPrompt{
		Signature: "function render(): ReactElement", Current: current,
		RepairGuidance: instruction,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"EXACT_MUTABLE_SOURCE_JSON:", current, "REQUIRED_SOURCE_TRANSFORMATION:", instruction} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("guided prompt omitted %q:\n%s", required, prompt)
		}
	}
	for _, forbidden := range []string{
		"LOCAL_BEHAVIOR:", "OBSERVED_FAILURE:", "REQUIRED_CHANGE:",
		"ONLY_AVAILABLE_DECLARATIONS:", "ALREADY_IN_SCOPE_IDENTIFIERS:",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("guided prompt retained analysis context %q:\n%s", forbidden, prompt)
		}
	}
}

func TestTypeScriptCorrectionPromptRejectsUnguidedAndMixedAuthority(t *testing.T) {
	t.Parallel()
	base := TypeScriptFragmentPrompt{
		Signature: "function render(): ReactElement",
		Current:   "function render(): ReactElement { return <div />; }",
	}
	for name, mutate := range map[string]func(*TypeScriptFragmentPrompt){
		"raw diagnostic": func(input *TypeScriptFragmentPrompt) {
			input.RequiredChange = "Replace the returned element."
			input.Diagnostic = "expected a button"
		},
		"missing guidance": func(*TypeScriptFragmentPrompt) {},
		"analysis context": func(input *TypeScriptFragmentPrompt) {
			input.RepairGuidance = "Replace the returned element."
			input.Available = "interface HiddenAnalysis {}"
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := base
			mutate(&input)
			if _, err := BuildTypeScriptFragmentPrompt(input); err == nil {
				t.Fatalf("accepted %s authority", name)
			}
		})
	}
}

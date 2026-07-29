package assemblyline

import (
	"strings"
	"testing"
)

func TestTypeScriptFragmentPromptContainsOnlyLocalAPIContext(t *testing.T) {
	prompt, err := BuildTypeScriptFragmentPrompt(TypeScriptFragmentPrompt{
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

func TestTypeScriptFragmentPromptRejectsOversizedEnvelopeSections(t *testing.T) {
	base := TypeScriptFragmentPrompt{
		Signature: "function apply(): void",
		Contract:  "Do the one supplied operation.",
	}
	for name, mutate := range map[string]func(*TypeScriptFragmentPrompt){
		"capabilities": func(input *TypeScriptFragmentPrompt) {
			input.Available = strings.Repeat("x", maxTypeScriptCapabilityBytes+1)
		},
		"current declaration": func(input *TypeScriptFragmentPrompt) {
			input.Contract = ""
			input.Current = strings.Repeat("x", maxTypeScriptCurrentDeclarationBytes+1)
		},
		"required change": func(input *TypeScriptFragmentPrompt) {
			input.Contract = ""
			input.Current = "function apply(): void { run(); }"
			input.RequiredChange = strings.Repeat("x", maxTypeScriptRequiredChangeBytes+1)
			input.Diagnostic = "rejected"
		},
		"diagnostic": func(input *TypeScriptFragmentPrompt) {
			input.Contract = ""
			input.Current = "function apply(): void { run(); }"
			input.RequiredChange = "Fix the rejected statement."
			input.Diagnostic = strings.Repeat("x", maxTypeScriptDiagnosticBytes+1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := base
			mutate(&input)
			if _, err := BuildTypeScriptFragmentPrompt(input); err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestTypeScriptFragmentPromptRejectsBloatedInitialEnvelope(t *testing.T) {
	_, err := BuildTypeScriptFragmentPrompt(TypeScriptFragmentPrompt{
		Signature: "function apply(): void",
		Contract:  strings.Repeat("c", 1500),
		Available: strings.Repeat("a", 1500),
	})
	if err == nil || !strings.Contains(err.Error(), "initial envelope") {
		t.Fatalf("error=%v", err)
	}
}

func TestTypeScriptCorrectionPromptOmitsSupersededBehaviorAndKeepsExactLocalFailure(t *testing.T) {
	prompt, err := BuildTypeScriptFragmentPrompt(TypeScriptFragmentPrompt{
		Signature:      "function render(value: Value): ReactElement",
		Available:      "interface Value { label: string }",
		Current:        "function render(value: Value): ReactElement { return <old-tag>{value.label}</old-tag>; }",
		RequiredChange: "Replace old-tag with a div whose className is label.",
		Diagnostic:     "expected undefined to be label",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"CURRENT_DECLARATION:", "REQUIRED_CHANGE:", "OBSERVED_FAILURE:", "interface Value",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("correction prompt omitted %q:\n%s", required, prompt)
		}
	}
	for _, forbidden := range []string{"LOCAL_BEHAVIOR:", "Render the value label", "workspace", "filename", "dependency graph"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("correction prompt leaked %q:\n%s", forbidden, prompt)
		}
	}
}

func TestTypeScriptCorrectionPromptRejectsReplayedInitialBehavior(t *testing.T) {
	t.Parallel()

	_, err := BuildTypeScriptFragmentPrompt(TypeScriptFragmentPrompt{
		Signature:      "function render(): ReactElement",
		Contract:       "Render the original product behavior.",
		Current:        "function render(): ReactElement { return <div />; }",
		RequiredChange: "Remove one invalid construct.",
		Diagnostic:     "invalid construct",
	})
	if err == nil || !strings.Contains(err.Error(), "cannot replay") {
		t.Fatalf("error=%v", err)
	}
}

func TestTypeScriptGenerationPromptRejectsCorrectionFieldsWithoutCurrentDeclaration(t *testing.T) {
	t.Parallel()

	_, err := BuildTypeScriptFragmentPrompt(TypeScriptFragmentPrompt{
		Signature:      "function render(): ReactElement",
		Contract:       "Render one control.",
		RequiredChange: "Change the control.",
		Diagnostic:     "control rejected",
	})
	if err == nil || !strings.Contains(err.Error(), "cannot carry correction") {
		t.Fatalf("error=%v", err)
	}
}

package assemblyline

import (
	"strings"
	"testing"
)

func TestGoFragmentCorrectionPromptIsNarrowAndPathFree(t *testing.T) {
	t.Parallel()
	input := FragmentCorrectionInput{
		Language: "go", Signature: "func Value() int",
		CurrentDeclaration: "func Value() int { return Helper() }",
		RequiredChange:     "Return the direct capability result as an integer.",
		Diagnostic:         "cannot use Helper() (value of type string) as int value in return statement",
		Capabilities:       []string{"func Helper() string"},
		PermittedSymbols:   []string{"Helper"},
	}
	prompt, err := BuildGoFragmentCorrectionPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		input.Signature, input.CurrentDeclaration, input.RequiredChange, input.Diagnostic,
		"func Helper() string", "Helper",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("correction prompt omitted %q: %s", required, prompt)
		}
	}
	for _, forbidden := range []string{"/workspace", "some/file.go", "ORIGINAL_REQUIREMENT", "REQUIREMENT_QUOTE"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("correction prompt leaked forbidden context %q: %s", forbidden, prompt)
		}
	}
}

func TestPortableGoFragmentCorrectionUsesTextResponse(t *testing.T) {
	t.Parallel()
	job, err := NewFragmentCorrectionJob(FragmentCorrectionInput{
		Language: "go", Signature: "func Value() int",
		CurrentDeclaration: "func Value() int { return 1 }",
		RequiredChange:     "Resolve only the observed local validation failure.",
		Diagnostic:         "returned value is unchanged",
	})
	if err != nil {
		t.Fatal(err)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if schema != nil {
		t.Fatalf("Go fragment correction unexpectedly returned schema %#v", schema)
	}
	if !strings.Contains(prompt, "CURRENT_DECLARATION") || strings.Contains(prompt, "EXACT_REQUIREMENT_QUOTE") {
		t.Fatalf("unexpected Go correction prompt: %s", prompt)
	}
}

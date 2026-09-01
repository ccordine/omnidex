package assemblyline

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestTypeScriptFragmentQuestionDoesNotDefineAResponsePacket(t *testing.T) {
	input := TypeScriptFragmentPrompt{
		Dialect:   "TypeScript",
		Signature: "function FormatCalendarDate(month: number, day: number): string",
		Contract:  "Return the supplied month and day with inert punctuation between them.",
	}
	prompt, err := BuildTypeScriptFragmentPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(prompt, input.Signature) != 1 {
		t.Fatalf("minimum lexical scope is absent or duplicated: %q", prompt)
	}
	if !strings.Contains(
		prompt,
		"This job is only the implementation itself; usage examples and documentation are not part of the task.",
	) {
		t.Fatalf("semantic job scope is absent: %q", prompt)
	}
	if !strings.Contains(prompt, "What TypeScript statements inside this function implement this behavior?") {
		t.Fatalf("function-local semantic responsibility is absent: %q", prompt)
	}
	for _, forbidden := range []string{
		"Return only", "raw code only", "response grammar", "response schema",
		"CODE_OWNED", "CURRENT_DECLARATION", "EXACT_SIGNATURE", "JSON",
		"control label", "AST", "preserve", "Omnidex will",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("initial question contains forbidden protocol text %q: %q", forbidden, prompt)
		}
	}
}

func TestTypeScriptFragmentQuestionDoesNotCarryCodeOwnedInteractionSurface(t *testing.T) {
	input := TypeScriptFragmentPrompt{
		Dialect:   "TypeScript React JSX",
		Signature: "function Feature(): JSX.Element",
		Contract:  "Provide the requested interaction.",
	}
	prompt, err := BuildTypeScriptFragmentPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"PUBLIC_INTERACTION_SURFACE", "END_PUBLIC_INTERACTION_SURFACE",
		"fragment-public-interaction-surface", "role_ordinal", "role_count", "value_kind",
		"Add item", "Current total", "screen.getByRole", "fireEvent", "toHaveTextContent",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("prompt exposed code-owned interaction state or test grammar %q: %q", forbidden, prompt)
		}
	}
}

func TestFragmentGenerationTypesHaveNoPublicInteractionSurfaceField(t *testing.T) {
	for _, value := range []any{FragmentGenerationInput{}, TypeScriptFragmentPrompt{}} {
		typeOf := reflect.TypeOf(value)
		if _, exists := typeOf.FieldByName("PublicInteractionSurface"); exists {
			t.Fatalf("%s still carries code-owned public interaction state", typeOf.Name())
		}
	}
}

func TestSourceCorrectionProjectsOnlyTheExactMutableSpan(t *testing.T) {
	base := "const total = left - right;\nreturn total;"
	start := strings.Index(base, "left - right")
	question := "Fix this expression so it adds left and right."
	defect, err := NewSourceBodyDefect(
		base, start, start+len("left - right"), question,
		errors.New("addition expression uses subtraction"),
	)
	if err != nil {
		t.Fatal(err)
	}
	correction, err := defect.Correction(base)
	if err != nil {
		t.Fatal(err)
	}
	modelInput, err := correction.ModelInput()
	if err != nil {
		t.Fatal(err)
	}
	if modelInput != question+"\n\nleft - right" {
		t.Fatalf("model input=%q", modelInput)
	}
	for _, forbidden := range []string{
		"const total", "return total", base, "preserve", "same job",
		"implementation body", "JSON", "schema",
	} {
		if strings.Contains(modelInput, forbidden) {
			t.Fatalf("correction exposed non-mutable state %q: %q", forbidden, modelInput)
		}
	}
	corrected, err := correction.Apply("left + right")
	if err != nil {
		t.Fatal(err)
	}
	if corrected != "const total = left + right;\nreturn total;" {
		t.Fatalf("code splice=%q", corrected)
	}
	evidence, err := correction.Evidence()
	if err != nil {
		t.Fatal(err)
	}
	if err := evidence.Validate(modelInput); err != nil {
		t.Fatal(err)
	}
}

func TestTypeScriptSourcePathBoundaryAcceptsRuntimePunctuationWithoutAcceptingPaths(t *testing.T) {
	accepted := []string{
		`return '/' + name;`,
		`return "/";`,
		`return '\/' + name;`,
		`return String(month) + String.fromCharCode(47) + String(day);`,
		`return String.fromCharCode(47) + name;`,
	}
	for index, source := range accepted {
		if err := ValidatePathFreeSourceModelContext("unrelated source fixture", source); err != nil {
			t.Fatalf("accepted fixture %d failed: %v", index, err)
		}
	}

	rejected := []string{
		`return "/etc/service";`,
		`return "./widget";`,
		`return "left/right";`,
		"// inspect /var/data\nreturn;",
	}
	for index, source := range rejected {
		if err := ValidatePathFreeSourceModelContext("forbidden source fixture", source); err == nil {
			t.Fatalf("rejected fixture %d unexpectedly crossed the path boundary: %q", index, source)
		}
	}
}

func TestPathBearingSpanCannotBecomeCorrectionModelContext(t *testing.T) {
	base := `return "/etc/service";`
	mutable := `"/etc/service"`
	start := strings.Index(base, mutable)
	defect, err := NewSourceBodyDefect(
		base, start, start+len(mutable),
		"Fix this value so it does not contain a filesystem identity.",
		errors.New("source contains a filesystem identity"),
	)
	if err != nil {
		t.Fatal(err)
	}
	correction, err := defect.Correction(base)
	if err != nil {
		t.Fatal(err)
	}
	modelInput, err := correction.ModelInput()
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePathFreeSourceModelContext("source correction", modelInput); err == nil {
		t.Fatal("path-bearing mutable bytes unexpectedly became legal model context")
	}
}

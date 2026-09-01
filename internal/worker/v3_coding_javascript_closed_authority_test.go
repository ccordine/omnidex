package worker

import (
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestJavaScriptSoleCallableCorrectionIsAppliedWithoutModelResolution(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "javascript", Dialect: "ECMAScript 2022",
		Signature:    "function Run(value)",
		Behavior:     "Resolve the supplied value.",
		Capabilities: []string{"function load(value)"},
	}
	body := "return import(value);"
	candidate, validatedBody, correction, err := validateDirectCodingLanguageBody(
		assemblyline.ArtifactIdentityProvenance{},
		directCodingLanguageGenerationJob{
			Subject: "source.javascript-sole-callable", Input: input,
			Validate: validateDirectCodingJavaScriptFragment,
		},
		body,
	)
	if err != nil {
		t.Fatal(err)
	}
	if correction != nil {
		t.Fatalf("sole code-owned callable reached model correction: %#v", correction)
	}
	if validatedBody != "return load(value);" {
		t.Fatalf("validated body=%q", validatedBody)
	}
	if !strings.Contains(candidate, validatedBody) {
		t.Fatalf("assembled candidate=%q; want corrected body", candidate)
	}
}

func TestJavaScriptCallableCorrectionExposesOnlyExactCallable(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "javascript", Dialect: "ECMAScript 2022",
		Signature: "function Run(value)", Behavior: "Resolve the supplied value.",
		Capabilities: []string{"function load(value)", "function resolve(value)"},
	}
	body := "return import(value);"
	_, validationErr := validateDirectCodingJavaScriptFragment(input, body)
	correction := requireSourceBodyCorrection(t, body, validationErr)
	if correction.Mutable() != "import" {
		t.Fatalf("mutable=%q; want callable token", correction.Mutable())
	}
	modelInput, err := correction.ModelInput()
	if err != nil {
		t.Fatal(err)
	}
	assertExactTokenChoiceInput(t, modelInput, body, input.Signature, "(value)", "return")
	corrected, err := correction.Apply("A")
	if err != nil {
		t.Fatal(err)
	}
	if corrected != "return load(value);" {
		t.Fatalf("callable splice changed accepted arguments: %q", corrected)
	}
}

func TestJavaScriptSolePropertyCorrectionIsAppliedWithoutModelResolution(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "javascript", Dialect: "ECMAScript 2022",
		Signature:    "function Read(record)",
		Behavior:     "Read one permitted property.",
		Capabilities: []string{`const SAFE_KEY = "value";`},
	}
	body := "return record.constructor;"
	_, validatedBody, correction, err := validateDirectCodingLanguageBody(
		assemblyline.ArtifactIdentityProvenance{},
		directCodingLanguageGenerationJob{
			Subject: "source.javascript-sole-property", Input: input,
			Validate: validateDirectCodingJavaScriptFragment,
		},
		body,
	)
	if err != nil {
		t.Fatal(err)
	}
	if correction != nil {
		t.Fatalf("sole code-owned property reached model correction: %#v", correction)
	}
	if validatedBody != "return record.value;" {
		t.Fatalf("validated body=%q", validatedBody)
	}
}

func TestJavaScriptSubscriptCorrectionExposesOnlyExactProperty(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "javascript", Dialect: "ECMAScript 2022",
		Signature: "function Read(record)", Behavior: "Read one permitted property.",
		Capabilities: []string{
			`const FIRST_KEY = "value";`,
			`const SECOND_KEY = "name";`,
		},
	}
	body := `return record["constructor"];`
	_, validationErr := validateDirectCodingJavaScriptFragment(input, body)
	correction := requireSourceBodyCorrection(t, body, validationErr)
	if correction.Mutable() != `"constructor"` {
		t.Fatalf("mutable=%q; want exact property literal", correction.Mutable())
	}
	modelInput, err := correction.ModelInput()
	if err != nil {
		t.Fatal(err)
	}
	assertExactTokenChoiceInput(t, modelInput, body, input.Signature, "record[", "]")
	corrected, err := correction.Apply("A")
	if err != nil {
		t.Fatal(err)
	}
	if corrected != `return record["name"];` {
		t.Fatalf("property splice changed accepted member bytes: %q", corrected)
	}
}

func TestJavaScriptClosedAuthorityWithoutCandidateFailsLoudly(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
	}{
		{name: "callable", body: "return import(value);"},
		{name: "property", body: "return value.constructor;"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			input := assemblyline.FragmentGenerationInput{
				Language: "javascript", Dialect: "ECMAScript 2022",
				Signature: "function Value(value)", Behavior: "Return one value.",
			}
			_, err := validateDirectCodingJavaScriptFragment(input, test.body)
			if err == nil {
				t.Fatal("unavailable authority unexpectedly passed")
			}
			var defect *assemblyline.SourceBodyDefect
			if errors.As(err, &defect) {
				t.Fatalf("zero candidates authorized raw model correction: %v", err)
			}
			if !strings.Contains(err.Error(), "no authorized replacement") {
				t.Fatalf("error=%v; want loud zero-candidate failure", err)
			}
		})
	}
}

func TestJavaScriptCompositePropertyFailsWithoutBroadCorrection(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "javascript", Dialect: "ECMAScript 2022",
		Signature: "function Read(record, prefix, suffix)",
		Behavior:  "Read one permitted property.",
		Capabilities: []string{
			`const FIRST_KEY = "value";`,
			`const SECOND_KEY = "name";`,
		},
	}
	body := "return record[prefix + suffix];"
	_, err := validateDirectCodingJavaScriptFragment(input, body)
	if err == nil {
		t.Fatal("unresolved composite property unexpectedly passed")
	}
	var defect *assemblyline.SourceBodyDefect
	if errors.As(err, &defect) {
		correction, correctionErr := defect.Correction(body)
		if correctionErr != nil {
			t.Fatal(correctionErr)
		}
		t.Fatalf("composite property authorized broad span %q", correction.Mutable())
	}
	if !strings.Contains(err.Error(), "exact computed property token") {
		t.Fatalf("error=%v; want loud exact-property failure", err)
	}
}

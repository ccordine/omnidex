package worker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestProductionLanguageValidatorsBindOneFailedNodeToReturnedBody(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    assemblyline.FragmentGenerationInput
		body     string
		validate directCodingLanguageFragmentValidator
	}{
		{
			name: "javascript",
			input: assemblyline.FragmentGenerationInput{
				Language: "javascript", Dialect: "ECMAScript 2022",
				Signature: "function Sum(left, right)",
				Behavior:  "Return the sum of left and right.",
			},
			body: "const total = missing + right;\nreturn total;", validate: validateDirectCodingJavaScriptFragment,
		},
		{
			name: "go",
			input: assemblyline.FragmentGenerationInput{
				Language: "go", Dialect: "Go 1.23",
				Signature: "func Sum(left, right int) int",
				Behavior:  "Return the sum of left and right.",
			},
			body: "total := missing + right\nreturn total", validate: validateDirectCodingGoFragment,
		},
		{
			name: "java",
			input: assemblyline.FragmentGenerationInput{
				Language: "java", Dialect: "Java 21",
				Signature: "static int Sum(int left, int right)",
				Behavior:  "Return the sum of left and right.",
			},
			body: "int total = missing + right;\nreturn total;", validate: validateDirectCodingJavaFragment,
		},
		{
			name: "rust",
			input: assemblyline.FragmentGenerationInput{
				Language: "rust", Dialect: "Rust 2021",
				Signature: "fn sum(left: i32, right: i32) -> i32",
				Behavior:  "Return the sum of left and right.",
			},
			body: "let total = missing + right;\ntotal", validate: validateDirectCodingRustFragment,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := test.validate(test.input, test.body)
			var defect *assemblyline.SourceBodyDefect
			if !errors.As(err, &defect) {
				t.Fatalf("validator error=%v; want exact source-body defect", err)
			}
			correction, err := defect.Correction(test.body)
			if err != nil {
				t.Fatal(err)
			}
			if correction.Mutable() != "missing" {
				t.Fatalf("mutable span=%q; want missing", correction.Mutable())
			}
			modelInput, err := correction.ModelInput()
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasSuffix(modelInput, "\n\nmissing") {
				t.Fatalf("correction input=%q", modelInput)
			}
			if !strings.Contains(modelInput, "A. ") || !strings.Contains(modelInput, "B. ") ||
				!strings.Contains(modelInput, "Answer with A or B.") {
				t.Fatalf("identifier correction is not an opaque choice: %q", modelInput)
			}
			for _, forbidden := range []string{
				test.input.Signature, test.body, "const total", "int total", "let total", "return total",
				"preserve", "implementation body", "JSON",
			} {
				if strings.Contains(modelInput, forbidden) {
					t.Fatalf("correction input exposed %q: %q", forbidden, modelInput)
				}
			}
		})
	}
}

func TestIdentifierChoicesExcludeLaterAndSiblingBindings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    assemblyline.FragmentGenerationInput
		body     string
		validate directCodingLanguageFragmentValidator
	}{
		{
			name: "javascript",
			input: assemblyline.FragmentGenerationInput{
				Language: "javascript", Dialect: "ECMAScript 2022",
				Signature: "function Value(input)", Behavior: "Return the supplied value.",
			},
			body:     "if (true) { const hidden = input; }\nconst result = missing;\nconst later = input;\nreturn result + later;",
			validate: validateDirectCodingJavaScriptFragment,
		},
		{
			name: "go",
			input: assemblyline.FragmentGenerationInput{
				Language: "go", Dialect: "Go 1.23",
				Signature: "func Value(input int) int", Behavior: "Return the supplied value.",
			},
			body:     "if true { hidden := input; _ = hidden }\nresult := missing\nlater := input\nreturn result + later",
			validate: validateDirectCodingGoFragment,
		},
		{
			name: "java",
			input: assemblyline.FragmentGenerationInput{
				Language: "java", Dialect: "Java 21",
				Signature: "static int Value(int input)", Behavior: "Return the supplied value.",
			},
			body:     "if (true) { int hidden = input; }\nint result = missing;\nint later = input;\nreturn result + later;",
			validate: validateDirectCodingJavaFragment,
		},
		{
			name: "rust",
			input: assemblyline.FragmentGenerationInput{
				Language: "rust", Dialect: "Rust 2021",
				Signature: "fn value(input: i32) -> i32", Behavior: "Return the supplied value.",
			},
			body:     "if true { let hidden = input; let _ = hidden; }\nlet result = missing;\nlet later = input;\nresult + later",
			validate: validateDirectCodingRustFragment,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := test.validate(test.input, test.body)
			var defect *assemblyline.SourceBodyDefect
			if !errors.As(err, &defect) {
				t.Fatalf("validator error=%v; want exact identifier defect", err)
			}
			correction, err := defect.Correction(test.body)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := correction.ModelInput(); err == nil ||
				!strings.Contains(err.Error(), "forbids a model call") {
				t.Fatalf("unavailable bindings created a model choice: %v", err)
			}
			corrected, resolved, err := correction.ApplySoleReplacement()
			if err != nil {
				t.Fatal(err)
			}
			if !resolved || !strings.Contains(corrected, "= input") &&
				!strings.Contains(corrected, ":= input") {
				t.Fatalf("sole replacement resolved=%v body=%q", resolved, corrected)
			}
		})
	}
}

func TestProductionTypeScriptPolicyBindsOnlyForbiddenIdentifier(t *testing.T) {
	t.Parallel()
	body := "const total = window + right;\nreturn total;"
	contract := assemblyline.TypeScriptFunctionContract{
		Signature: "function Sum(left: number, right: number): number",
		Policy: assemblyline.SourceFunctionPolicy{
			ForbiddenIdentifiers: []string{"window"},
		},
	}
	_, err := assemblyline.ParseTypeScriptFunctionBody(contract, body)
	var defect *assemblyline.SourceBodyDefect
	if !errors.As(err, &defect) {
		t.Fatalf("TypeScript policy error=%v; want exact source-body defect", err)
	}
	failedStart, failedEnd, err := defect.MutableRange(body)
	if err != nil {
		t.Fatal(err)
	}
	input := assemblyline.FragmentGenerationInput{
		Language: "typescript", Dialect: "TypeScript",
		Signature: contract.Signature,
	}
	replacements, err := directCodingTypeScriptIdentifierChoices(
		input, body, body[failedStart:failedEnd], failedStart, failedEnd, false,
		contract.Policy,
	)
	if err != nil {
		t.Fatal(err)
	}
	defect, err = defect.WithIdentifierReplacements(replacements)
	if err != nil {
		t.Fatal(err)
	}
	correction, err := defect.Correction(body)
	if err != nil {
		t.Fatal(err)
	}
	if correction.Mutable() != "window" {
		t.Fatalf("mutable span=%q; want window", correction.Mutable())
	}
	modelInput, err := correction.ModelInput()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(modelInput, "A. TypeScript function parameter named \"left\"") ||
		!strings.Contains(modelInput, "B. TypeScript function parameter named \"right\"") ||
		!strings.HasSuffix(modelInput, "\n\nwindow") {
		t.Fatalf("correction input=%q", modelInput)
	}
	for _, forbidden := range []string{contract.Signature, body, "const total", "return total"} {
		if strings.Contains(modelInput, forbidden) {
			t.Fatalf("correction input exposed %q: %q", forbidden, modelInput)
		}
	}
}

func TestTypeScriptIdentifierChoicesExcludeLaterAndSiblingBindings(t *testing.T) {
	t.Parallel()
	body := "if (true) { const hidden = input; }\nconst result = window;\nconst later = input;\nreturn result + later;"
	contract := assemblyline.TypeScriptFunctionContract{
		Signature: "function Value(input: number): number",
		Policy: assemblyline.SourceFunctionPolicy{
			ForbiddenIdentifiers: []string{"window"},
		},
	}
	_, err := assemblyline.ParseTypeScriptFunctionBody(contract, body)
	var defect *assemblyline.SourceBodyDefect
	if !errors.As(err, &defect) {
		t.Fatalf("TypeScript policy error=%v; want exact identifier defect", err)
	}
	start, end, err := defect.MutableRange(body)
	if err != nil {
		t.Fatal(err)
	}
	input := assemblyline.FragmentGenerationInput{
		Language: "typescript", Dialect: "TypeScript", Signature: contract.Signature,
	}
	replacements, err := directCodingTypeScriptIdentifierChoices(
		input, body, body[start:end], start, end, false, contract.Policy,
	)
	if err != nil {
		t.Fatal(err)
	}
	defect, err = defect.WithIdentifierReplacements(replacements)
	if err != nil {
		t.Fatal(err)
	}
	correction, err := defect.Correction(body)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := correction.ModelInput(); err == nil ||
		!strings.Contains(err.Error(), "forbids a model call") {
		t.Fatalf("unavailable TypeScript bindings created a model choice: %v", err)
	}
	corrected, resolved, err := correction.ApplySoleReplacement()
	if err != nil {
		t.Fatal(err)
	}
	if !resolved || !strings.Contains(corrected, "const result = input;") {
		t.Fatalf("sole TypeScript replacement resolved=%v body=%q", resolved, corrected)
	}
}

func TestProductionCompositeParserErrorFailsWithoutCorrection(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "javascript", Dialect: "ECMAScript 2022",
		Signature: "function Sum(left, right)",
		Behavior:  "Return the sum of left and right.",
	}
	body := "return left § right;"
	_, err := validateDirectCodingJavaScriptFragment(input, body)
	var defect *assemblyline.SourceBodyDefect
	if err == nil {
		t.Fatal("composite parser error unexpectedly passed")
	}
	if errors.As(err, &defect) {
		t.Fatalf("composite parser error authorized model correction: %v", err)
	}
	if !strings.Contains(err.Error(), "exact non-empty parser-error leaf") {
		t.Fatalf("parser error=%v; want loud exact-leaf failure", err)
	}
}

func TestProductionJavaScriptCorrectionSplicesWithoutResendingAcceptedSource(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "javascript", Dialect: "ECMAScript 2022",
		Signature: "function Sum(left, right)",
		Behavior:  "Return the sum of left and right.",
	}
	initialBody := "const total = missing + right;\nreturn total;"
	var correctionInput string
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: assemblyline.MaxSourceBodyAttempts,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			return exactSourceBodyTestResult(t, job, initialBody), nil
		},
		Correct: func(
			job assemblyline.PortableJob,
			model string,
			correction assemblyline.SourceBodyCorrection,
		) (assemblyline.PortableResult, error) {
			if model != "fixture-model" || correction.Mutable() != "missing" {
				return assemblyline.PortableResult{}, fmt.Errorf(
					"unexpected correction model=%q mutable=%q", model, correction.Mutable(),
				)
			}
			var err error
			correctionInput, err = correction.ModelInput()
			if err != nil {
				return assemblyline.PortableResult{}, err
			}
			return exactSourceBodyTestResult(t, job, "A"), nil
		},
		Release: func(assemblyline.PortableJob) error { return nil },
		Finalize: func(
			assemblyline.PortableJob,
			assemblyline.PortableResult,
			error,
		) error {
			return nil
		},
	}
	validated, err := runDirectCodingLanguageFragmentWorker(
		runtime,
		"fixture-model",
		directCodingLanguageGenerationJob{
			Subject: "source.production-span", Input: input,
			Validate: validateDirectCodingJavaScriptFragment,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(correctionInput, "A. JavaScript lexically in-scope value named \"left\"") ||
		!strings.Contains(correctionInput, "B. JavaScript lexically in-scope value named \"right\"") ||
		!strings.HasSuffix(correctionInput, "\n\nmissing") {
		t.Fatalf("correction input=%q", correctionInput)
	}
	want := "function Sum(left, right) {\nconst total = left + right;\nreturn total;\n}"
	if validated != want {
		t.Fatalf("validated source=%q; want %q", validated, want)
	}
	for _, accepted := range []string{"const total = ", " + right;", "\nreturn total;"} {
		if !strings.Contains(validated, accepted) {
			t.Fatalf("accepted source was not preserved: %q", validated)
		}
	}
}

func TestProductionSoleIdentifierReplacementMakesZeroCorrectionCalls(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "javascript", Dialect: "ECMAScript 2022",
		Signature: "function Identity(value)",
		Behavior:  "Return the supplied value.",
	}
	correctionCalls := 0
	accepted := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: assemblyline.MaxSourceBodyAttempts,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			return exactSourceBodyTestResult(t, job, "return missing;"), nil
		},
		Correct: func(
			assemblyline.PortableJob,
			string,
			assemblyline.SourceBodyCorrection,
		) (assemblyline.PortableResult, error) {
			correctionCalls++
			return assemblyline.PortableResult{}, fmt.Errorf("sole option reached the provider")
		},
		Release: func(assemblyline.PortableJob) error { return nil },
		Finalize: func(
			_ assemblyline.PortableJob,
			_ assemblyline.PortableResult,
			validationErr error,
		) error {
			if validationErr != nil {
				return fmt.Errorf("sole code-owned splice was rejected: %w", validationErr)
			}
			accepted++
			return nil
		},
	}
	validated, err := runDirectCodingLanguageFragmentWorker(
		runtime,
		"fixture-model",
		directCodingLanguageGenerationJob{
			Subject: "source.sole-identifier", Input: input,
			Validate: validateDirectCodingJavaScriptFragment,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if correctionCalls != 0 || accepted != 1 {
		t.Fatalf("correction calls=%d accepted=%d", correctionCalls, accepted)
	}
	want := "function Identity(value) {\nreturn value;\n}"
	if validated != want {
		t.Fatalf("validated source=%q; want %q", validated, want)
	}
}

func TestGoMultipleUnresolvedOccurrencesExposeOnlyFirstExactSpan(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "go", Dialect: "Go 1.23",
		Signature: "func Sum(left, right int) int",
		Behavior:  "Return the sum of left and right.",
	}
	_, err := validateDirectCodingGoFragment(input, "return missing + missing")
	var defect *assemblyline.SourceBodyDefect
	if !errors.As(err, &defect) {
		t.Fatalf("first unresolved reference was not located: %v", err)
	}
	correction, err := defect.Correction("return missing + missing")
	if err != nil {
		t.Fatal(err)
	}
	if correction.Mutable() != "missing" {
		t.Fatalf("mutable span=%q; want first missing", correction.Mutable())
	}
	corrected, err := correction.Apply("A")
	if err != nil {
		t.Fatal(err)
	}
	if corrected != "return left + missing" {
		t.Fatalf("corrected body=%q", corrected)
	}
}

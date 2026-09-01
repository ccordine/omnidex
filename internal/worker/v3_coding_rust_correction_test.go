package worker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestRustMacroCorrectionPreservesInvocationArguments(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "rust", Dialect: "Rust 2021",
		Signature: "fn render(input: &str) -> String",
		Behavior:  "Render the supplied value.",
	}
	body := `missing!("{}", input)`
	correction := exactRustCorrection(t, input, body, "missing")
	modelInput, err := correction.ModelInput()
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`!("{}", input)`, body, input.Signature} {
		if strings.Contains(modelInput, forbidden) {
			t.Fatalf("macro correction exposed code-owned bytes %q: %q", forbidden, modelInput)
		}
	}
	if _, err := correction.Apply("format"); err == nil {
		t.Fatal("code-owned macro name was accepted instead of an opaque choice ID")
	}
	corrected, err := correction.Apply("A")
	if err != nil {
		t.Fatal(err)
	}
	if corrected != `format!("{}", input)` {
		t.Fatalf("corrected body=%q", corrected)
	}
}

func TestRustPathRootCorrectionPreservesSuffixAndArguments(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "rust", Dialect: "Rust 2021",
		Signature:    "fn convert(input: i32) -> i32",
		Behavior:     "Convert the supplied value.",
		Capabilities: []string{"struct Allowed;"},
	}
	body := "missing::convert(input + 1)"
	correction := exactRustCorrection(t, input, body, "missing")
	modelInput, err := correction.ModelInput()
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"::convert(input + 1)", body, input.Signature} {
		if strings.Contains(modelInput, forbidden) {
			t.Fatalf("path correction exposed code-owned bytes %q: %q", forbidden, modelInput)
		}
	}
	if _, err := correction.Apply("Allowed"); err == nil {
		t.Fatal("code-owned path root was accepted instead of an opaque choice ID")
	}
	corrected, err := correction.Apply("A")
	if err != nil {
		t.Fatal(err)
	}
	if corrected != "Allowed::convert(input + 1)" {
		t.Fatalf("corrected body=%q", corrected)
	}
}

func TestRustForbiddenPathComponentIsOneExactZeroChoiceLeaf(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "rust", Dialect: "Rust 2021",
		Signature: "fn value() -> i32", Behavior: "Return one value.",
	}
	body := "Option::std::None"
	_, err := validateDirectCodingRustFragment(input, body)
	if err == nil || !strings.Contains(err.Error(), "path component std") ||
		!strings.Contains(err.Error(), "no authorized replacement") {
		t.Fatalf("zero-choice path component did not fail loudly and exactly: %v", err)
	}
}

func TestRustSoleCallableReplacementMakesZeroModelCalls(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "rust", Dialect: "Rust 2021",
		Signature:    "fn run(input: i32) -> i32",
		Behavior:     "Return the available result.",
		Capabilities: []string{"fn ready() -> i32"},
	}
	correctionCalls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: assemblyline.MaxSourceBodyAttempts,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			return exactSourceBodyTestResult(t, job, "missing()"), nil
		},
		Correct: func(
			assemblyline.PortableJob,
			string,
			assemblyline.SourceBodyCorrection,
		) (assemblyline.PortableResult, error) {
			correctionCalls++
			return assemblyline.PortableResult{}, fmt.Errorf("sole callable reached the provider")
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
			Subject: "source.rust-sole-callable", Input: input,
			Validate: validateDirectCodingRustFragment,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if correctionCalls != 0 {
		t.Fatalf("correction calls=%d; want zero", correctionCalls)
	}
	want := "fn run(input: i32) -> i32 {\nready()\n}"
	if validated != want {
		t.Fatalf("validated source=%q; want %q", validated, want)
	}
}

func TestRustTypeCorrectionTargetsOnlyTypeToken(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "rust", Dialect: "Rust 2021",
		Signature: "fn convert(input: i32) -> i32",
		Behavior:  "Convert the supplied value.",
	}
	body := "let value: Missing = input;\nvalue"
	correction := exactRustCorrection(t, input, body, "Missing")
	modelInput, err := correction.ModelInput()
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"let value:", "= input", "\nvalue"} {
		if strings.Contains(modelInput, forbidden) {
			t.Fatalf("type correction exposed code-owned bytes %q: %q", forbidden, modelInput)
		}
	}
}

func TestRustWholeBodyFailureDoesNotInvokeCorrection(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "rust", Dialect: "Rust 2021",
		Signature: "fn value() -> i32", Behavior: "Return one safe value.",
	}
	correctionCalls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: assemblyline.MaxSourceBodyAttempts,
		Execute: func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
			if model != "fixture-model" {
				return assemblyline.PortableResult{}, fmt.Errorf("initial model=%q", model)
			}
			return exactSourceBodyTestResult(t, job, "unsafe { 1 }"), nil
		},
		Correct: func(
			assemblyline.PortableJob,
			string,
			assemblyline.SourceBodyCorrection,
		) (assemblyline.PortableResult, error) {
			correctionCalls++
			return assemblyline.PortableResult{}, fmt.Errorf("whole-body correction reached provider")
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
	_, err := runDirectCodingLanguageFragmentWorker(
		runtime,
		"fixture-model",
		directCodingLanguageGenerationJob{
			Subject: "source.rust-whole-body-forbidden", Input: input,
			Validate: validateDirectCodingRustFragment,
		},
	)
	if err == nil || !strings.Contains(err.Error(), "no code-proven mutable source span") {
		t.Fatalf("whole-body validation error=%v", err)
	}
	if correctionCalls != 0 {
		t.Fatalf("whole-body correction calls=%d; want zero", correctionCalls)
	}
}

func TestRustCorrectionChoicesStayInsideTheFailedNamespace(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "rust", Dialect: "Rust 2021",
		Signature: "fn execute(input: i32, spare: i32) -> i32",
		Behavior:  "Use the available authority.",
		Capabilities: []string{
			"fn ready(value: i32) -> i32",
			"struct KnownType;",
			"const KNOWN_VALUE: i32 = 1;",
			"macro_rules! known_macro { ($value:expr) => { $value }; }",
		},
	}
	tests := []struct {
		name      string
		body      string
		want      string
		forbidden []string
	}{
		{
			name: "macro", body: "missing!(input)", want: "known_macro",
			forbidden: []string{"ready", "KnownType", "KNOWN_VALUE"},
		},
		{
			name: "function", body: "missing(input)", want: "ready",
			forbidden: []string{"known_macro", "KnownType", "KNOWN_VALUE"},
		},
		{
			name: "path root", body: "missing::value(input)", want: "KnownType",
			forbidden: []string{"known_macro", "ready", "KNOWN_VALUE"},
		},
		{
			name: "type", body: "let value: missing = input;\nvalue", want: "KnownType",
			forbidden: []string{"known_macro", "ready", "KNOWN_VALUE"},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			correction := exactRustCorrection(t, input, test.body, "missing")
			modelInput, err := correction.ModelInput()
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(modelInput, test.want) {
				t.Fatalf("choice input=%q; want namespace candidate %q", modelInput, test.want)
			}
			for _, forbidden := range test.forbidden {
				if strings.Contains(modelInput, forbidden) {
					t.Fatalf("choice input crossed namespace into %q: %q", forbidden, modelInput)
				}
			}
		})
	}
}

func TestRustScopedTypeCorrectionPreservesPathSuffix(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "rust", Dialect: "Rust 2021",
		Signature:    "fn convert(input: i32) -> i32",
		Behavior:     "Convert the supplied value.",
		Capabilities: []string{"struct Allowed;"},
	}
	body := "let value: missing::Item = input;\nvalue"
	correction := exactRustCorrection(t, input, body, "missing")
	corrected, err := correction.Apply("A")
	if err != nil {
		t.Fatal(err)
	}
	if corrected != "let value: Allowed::Item = input;\nvalue" {
		t.Fatalf("corrected body=%q", corrected)
	}
}

func exactRustCorrection(
	t *testing.T,
	input assemblyline.FragmentGenerationInput,
	body string,
	wantMutable string,
) assemblyline.SourceBodyCorrection {
	t.Helper()
	_, err := validateDirectCodingRustFragment(input, body)
	var defect *assemblyline.SourceBodyDefect
	if !errors.As(err, &defect) {
		t.Fatalf("validator error=%v; want exact source-body defect", err)
	}
	correction, err := defect.Correction(body)
	if err != nil {
		t.Fatal(err)
	}
	if correction.Mutable() != wantMutable {
		t.Fatalf("mutable span=%q; want %q", correction.Mutable(), wantMutable)
	}
	return correction
}

package worker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestLanguageWorkerExtractsRedundantDeclarationsWithoutCorrection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    assemblyline.FragmentGenerationInput
		response string
		validate directCodingLanguageFragmentValidator
	}{
		{
			name: "go",
			input: assemblyline.FragmentGenerationInput{
				Language: "go", Dialect: "Go 1.23",
				Signature: "func Sum(left, right int) int", Behavior: "Return the sum.",
			},
			response: "func Wrong(unused string) string { return left + right }",
			validate: validateDirectCodingGoFragment,
		},
		{
			name: "typescript",
			input: assemblyline.FragmentGenerationInput{
				Language: "typescript", Dialect: "TypeScript",
				Signature: "function Sum(left: number, right: number): number", Behavior: "Return the sum.",
			},
			response: "Here is the implementation.\n```typescript\nfunction Wrong(unused: string): string { return left + right; }\n```",
			validate: func(input assemblyline.FragmentGenerationInput, body string) (string, error) {
				fragment, err := assemblyline.ParseTypeScriptFunctionBody(
					assemblyline.TypeScriptFunctionContract{Signature: input.Signature}, body,
				)
				return strings.TrimSpace(fragment.Source), err
			},
		},
		{
			name: "javascript",
			input: assemblyline.FragmentGenerationInput{
				Language: "javascript", Dialect: "ECMAScript 2022",
				Signature: "function Sum(left, right)", Behavior: "Return the sum.",
			},
			response: "function Wrong(unused) { return left + right; }",
			validate: validateDirectCodingJavaScriptFragment,
		},
		{
			name: "java",
			input: assemblyline.FragmentGenerationInput{
				Language: "java", Dialect: "Java 21",
				Signature: "static int Sum(int left, int right)", Behavior: "Return the sum.",
			},
			response: "private String Wrong(String unused) { return left + right; }",
			validate: validateDirectCodingJavaFragment,
		},
		{
			name: "rust",
			input: assemblyline.FragmentGenerationInput{
				Language: "rust", Dialect: "Rust 2021",
				Signature: "fn sum(left: i32, right: i32) -> i32", Behavior: "Return the sum.",
			},
			response: "pub fn wrong(unused: i32) -> i32 { left + right }",
			validate: validateDirectCodingRustFragment,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			corrections := 0
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: assemblyline.MaxSourceBodyAttempts,
				Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
					return exactSourceBodyTestResult(t, job, test.response), nil
				},
				Correct: func(
					assemblyline.PortableJob,
					string,
					assemblyline.SourceBodyCorrection,
				) (assemblyline.PortableResult, error) {
					corrections++
					return assemblyline.PortableResult{}, fmt.Errorf("unexpected correction")
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
			source, err := runDirectCodingLanguageFragmentWorker(
				runtime,
				"fixture-model",
				directCodingLanguageGenerationJob{
					Subject: test.name, Input: test.input, Validate: test.validate,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if corrections != 0 {
				t.Fatalf("redundant declaration triggered %d corrections", corrections)
			}
			if !strings.Contains(source, test.input.Signature) ||
				strings.Contains(source, "Wrong") || strings.Contains(source, "wrong") {
				t.Fatalf("code-owned %s source=%q", test.name, source)
			}
		})
	}
}

func TestLanguageWorkerDoesNotCorrectAmbiguousFencedProse(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "javascript", Dialect: "ECMAScript 2022",
		Signature: "function Value()", Behavior: "Return one.",
	}
	response := "```javascript\nfunction One() { return 1; }\n```\n```javascript\nfunction Two() { return 2; }\n```"
	corrections := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: assemblyline.MaxSourceBodyAttempts,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			return exactSourceBodyTestResult(t, job, response), nil
		},
		Correct: func(
			assemblyline.PortableJob,
			string,
			assemblyline.SourceBodyCorrection,
		) (assemblyline.PortableResult, error) {
			corrections++
			return assemblyline.PortableResult{}, fmt.Errorf("unexpected correction")
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
			Subject: "ambiguous", Input: input, Validate: validateDirectCodingJavaScriptFragment,
		},
	)
	if err == nil || !strings.Contains(err.Error(), "2 fenced regions") {
		t.Fatalf("ambiguous worker extraction error=%v", err)
	}
	if corrections != 0 {
		t.Fatalf("ambiguous response triggered %d corrections", corrections)
	}
	var defect *assemblyline.SourceBodyDefect
	if errors.As(err, &defect) {
		t.Fatalf("ambiguous response acquired correction authority: %v", err)
	}
}

func TestLanguageWorkerExtractsFencedOpenSpanCorrection(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "javascript", Dialect: "ECMAScript 2022",
		Signature: "function Sum(left, right)", Behavior: "Return the sum.",
	}
	corrections := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: assemblyline.MaxSourceBodyAttempts,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			return exactSourceBodyTestResult(t, job, "return left + #;"), nil
		},
		Correct: func(
			job assemblyline.PortableJob,
			_ string,
			correction assemblyline.SourceBodyCorrection,
		) (assemblyline.PortableResult, error) {
			corrections++
			if correction.Mutable() != "#" {
				t.Fatalf("mutable correction=%q; want #", correction.Mutable())
			}
			return exactSourceBodyTestResult(
				t, job, "The replacement follows.\n```javascript\nright\n```",
			), nil
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
	source, err := runDirectCodingLanguageFragmentWorker(
		runtime,
		"fixture-model",
		directCodingLanguageGenerationJob{
			Subject: "fenced-correction", Input: input,
			Validate: validateDirectCodingJavaScriptFragment,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if corrections != 1 || !strings.Contains(source, "return left + right;") ||
		strings.Contains(source, "replacement follows") {
		t.Fatalf("corrections=%d source=%q", corrections, source)
	}
}

package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/gofragment"
)

func TestLanguageGenerationFinalizesAndReturnsExactDeclarationProjection(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name     string
		raw      string
		want     string
		input    assemblyline.FragmentGenerationInput
		project  directCodingLanguageFragmentProjector
		validate directCodingLanguageFragmentValidator
	}{
		{
			name: "go fenced response",
			raw:  " \n```go\nfunc Value() int { return 2 }\n```\n",
			want: "func Value() int { return 2 }",
			input: assemblyline.FragmentGenerationInput{
				Language: "go", Dialect: "Go 1.24", Signature: "func Value() int",
				Behavior: "Return two.",
			},
			project: projectDirectCodingGoFragment, validate: validateDirectCodingGoFragment,
		},
		{
			name: "javascript CRLF response",
			raw:  " \r\nfunction value() {\r\n  return 2;\r\n}\r\n ",
			want: "function value() {\r\n  return 2;\r\n}",
			input: assemblyline.FragmentGenerationInput{
				Language: "javascript", Dialect: "ECMAScript 2022",
				Signature: "function value()", Behavior: "Return two.",
			},
			project:  assemblyline.ProjectJavaScriptFragment,
			validate: validateDirectCodingJavaScriptFragment,
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			finalized := false
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 1,
				Execute: testPortableExecutor(func(
					_ string, _ string, _ string, _ map[string]any,
				) (string, error) {
					return fixture.raw, nil
				}),
				Finalize: func(
					_ assemblyline.PortableJob,
					result assemblyline.PortableResult,
					validationErr error,
				) error {
					if validationErr != nil || result.Candidate != fixture.raw ||
						result.Projection == nil ||
						result.Projection.Kind != assemblyline.PortableResultProjectionSourceDeclaration ||
						result.Projection.Source != fixture.want ||
						result.Projection.Source != fixture.raw[result.Projection.StartByte:result.Projection.EndByte] {
						t.Fatalf("finalized result=%+v validation=%v", result, validationErr)
					}
					finalized = true
					return nil
				},
			}
			got, err := runDirectCodingLanguageFragmentWorker(
				runtime, "fragment-model", directCodingLanguageGenerationJob{
					Subject: "opaque-block", Input: fixture.input,
					Project: fixture.project, Validate: fixture.validate,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if got != fixture.want || !finalized {
				t.Fatalf("source=%q finalized=%t", got, finalized)
			}
		})
	}
}

func TestLanguageCorrectionFinalizesAndReturnsExactProjectedResponseSpan(t *testing.T) {
	t.Parallel()
	const current = "func Value() int { return 1 }"
	const raw = " \n```go\nfunc Value() int { return 2 }\n```\n "
	const want = "func Value() int { return 2 }"
	contract := gofragment.Contract{Signature: "func Value() int", Current: current}
	finalized := false
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: testPortableExecutor(func(
			_ string, _ string, _ string, _ map[string]any,
		) (string, error) {
			return raw, nil
		}),
		Finalize: func(
			_ assemblyline.PortableJob,
			result assemblyline.PortableResult,
			validationErr error,
		) error {
			if validationErr != nil || result.Candidate != raw || result.Projection == nil ||
				result.Projection.Kind != assemblyline.PortableResultProjectionSourceDeclaration ||
				result.Projection.Source != want ||
				result.Projection.DiscardedBytes != len(raw)-len(want) ||
				result.Projection.Source != raw[result.Projection.StartByte:result.Projection.EndByte] {
				t.Fatalf("finalized result=%+v validation=%v", result, validationErr)
			}
			finalized = true
			return nil
		},
	}
	got, err := runDirectCodingLanguageCorrection(
		runtime, "executor", "opaque-block", current, "Return two.",
		"go",
		func(candidate string) (string, error) {
			return gofragment.ParseFunction(contract, candidate)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got != want || !finalized || !strings.Contains(raw, got) {
		t.Fatalf("source=%q finalized=%t", got, finalized)
	}
}

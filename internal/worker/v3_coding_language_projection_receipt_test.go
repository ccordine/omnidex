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
			name: "go exact response",
			raw:  "func Value() int { return 2 }",
			want: "func Value() int { return 2 }",
			input: assemblyline.FragmentGenerationInput{
				Language: "go", Dialect: "Go 1.24", Signature: "func Value() int",
				Behavior: "Return two.",
			},
			project: projectDirectCodingGoFragment, validate: validateDirectCodingGoFragment,
		},
		{
			name: "javascript exact CRLF response",
			raw:  "function value() {\r\n  return 2;\r\n}",
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
					_ string, _ string, _ string) (string, error) {
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
						result.Projection.StartByte != 0 ||
						result.Projection.EndByte != len(fixture.raw) ||
						result.Projection.DiscardedBytes != 0 {
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

func TestLanguageCorrectionFinalizesAndReturnsCompleteExactResponse(t *testing.T) {
	t.Parallel()
	const current = "func Value() int { return 1 }"
	const raw = "func Value() int { return 2 }"
	const want = raw
	contract := gofragment.Contract{Signature: "func Value() int", Current: current}
	finalized := false
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: testPortableExecutor(func(
			_ string, _ string, _ string) (string, error) {
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
				result.Projection.StartByte != 0 ||
				result.Projection.EndByte != len(raw) ||
				result.Projection.DiscardedBytes != 0 {
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

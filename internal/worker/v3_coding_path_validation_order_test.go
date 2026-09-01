package worker

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTypeScriptBodyAllowsSingletonSlashLiteral(t *testing.T) {
	t.Parallel()
	input := typeScriptPathBoundaryInput()
	body := `return '/' + name;`
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: assemblyline.MaxSourceBodyAttempts,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			return exactSourceBodyTestResult(t, job, body), nil
		},
		Correct: func(
			assemblyline.PortableJob,
			string,
			assemblyline.SourceBodyCorrection,
		) (assemblyline.PortableResult, error) {
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

	got, err := runDirectCodingLanguageFragmentWorker(
		runtime,
		"fixture-model",
		typeScriptPathBoundaryJob(input),
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "function Join(name: string): string {\nreturn '/' + name;\n}"
	if got != want {
		t.Fatalf("validated source=%q; want %q", got, want)
	}
}

func TestTypeScriptDeclarationNoiseIsDiscardedBeforePathValidation(t *testing.T) {
	t.Parallel()
	input := typeScriptPathBoundaryInput()
	response := `function Join(name: string): string { return '/etc/' + name; }`
	corrections := 0
	var validationDetail string
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
			_ assemblyline.PortableJob,
			_ assemblyline.PortableResult,
			validationErr error,
		) error {
			if validationErr != nil {
				validationDetail = validationErr.Error()
			}
			return nil
		},
	}

	_, err := runDirectCodingLanguageFragmentWorker(
		runtime,
		"fixture-model",
		typeScriptPathBoundaryJob(input),
	)
	if err == nil {
		t.Fatal("path-bearing implementation unexpectedly passed")
	}
	for label, value := range map[string]string{
		"recorded validation": validationDetail,
		"worker failure":      err.Error(),
	} {
		if !strings.Contains(value, "filesystem identity") {
			t.Fatalf("%s=%q; want validation of the extracted body", label, value)
		}
		if strings.Contains(value, "repeated the code-owned declaration") {
			t.Fatalf("%s retained a model-owned declaration duty: %q", label, value)
		}
	}
	if corrections != 0 {
		t.Fatalf("structurally broad response triggered %d corrections", corrections)
	}
}

func TestTypeScriptValidPathLiteralRemainsRejected(t *testing.T) {
	t.Parallel()
	input := typeScriptPathBoundaryInput()
	response := `return '/etc/' + name;`
	corrections := 0
	var validationDetail string
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
			_ assemblyline.PortableJob,
			_ assemblyline.PortableResult,
			validationErr error,
		) error {
			if validationErr != nil {
				validationDetail = validationErr.Error()
			}
			return nil
		},
	}

	_, err := runDirectCodingLanguageFragmentWorker(
		runtime,
		"fixture-model",
		typeScriptPathBoundaryJob(input),
	)
	if err == nil || !strings.Contains(err.Error(), "filesystem identity") {
		t.Fatalf("path-bearing body error=%v", err)
	}
	if !strings.Contains(validationDetail, "filesystem identity") {
		t.Fatalf("recorded validation=%q; want path rejection", validationDetail)
	}
	if corrections != 0 {
		t.Fatalf("path-bearing body triggered %d corrections", corrections)
	}
}

func typeScriptPathBoundaryInput() assemblyline.FragmentGenerationInput {
	return assemblyline.FragmentGenerationInput{
		Language:  "typescript",
		Dialect:   "TypeScript",
		Signature: "function Join(name: string): string",
		Behavior:  "Prefix the supplied name with slash punctuation.",
	}
}

func typeScriptPathBoundaryJob(
	input assemblyline.FragmentGenerationInput,
) directCodingLanguageGenerationJob {
	return directCodingLanguageGenerationJob{
		Subject: "source.path-boundary",
		Input:   input,
		Validate: func(
			_ assemblyline.FragmentGenerationInput,
			body string,
		) (string, error) {
			fragment, err := assemblyline.ParseTypeScriptFunctionBody(
				assemblyline.TypeScriptFunctionContract{Signature: input.Signature},
				body,
			)
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(fragment.Source), nil
		},
	}
}

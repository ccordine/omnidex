package worker

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTypeScriptWorkerQualifiesOrdinaryBodyWithoutDeclaration(t *testing.T) {
	const signature = "function Add(left: number, right: number): number"
	const body = "return left + right;"
	executions, corrections, finalizations := 0, 0, 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: assemblyline.MaxSourceBodyAttempts,
		Execute: func(
			job assemblyline.PortableJob,
			model string,
		) (assemblyline.PortableResult, error) {
			executions++
			if model != "fixture-model" {
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected model %q", model)
			}
			prompt, err := assemblyline.RenderPortableJob(job)
			if err != nil {
				return assemblyline.PortableResult{}, err
			}
			if strings.Count(prompt, signature) != 1 || !strings.Contains(
				prompt, "What TypeScript statements inside this function implement this behavior?",
			) {
				return assemblyline.PortableResult{}, fmt.Errorf(
					"prompt does not define one function-local statements job: %q", prompt,
				)
			}
			lower := strings.ToLower(prompt)
			for _, forbidden := range []string{
				"declaration", "reproduce", "repeat the", "preserve",
				"response schema", "response format", "response packet",
				"return only", "raw code", "ast",
			} {
				if strings.Contains(lower, forbidden) {
					return assemblyline.PortableResult{}, fmt.Errorf(
						"prompt contains declaration/protocol instruction %q: %q", forbidden, prompt,
					)
				}
			}
			if strings.Contains(body, signature) {
				return assemblyline.PortableResult{}, fmt.Errorf("ordinary body repeats code-owned signature")
			}
			return exactSourceBodyTestResult(t, job, body), nil
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
			result assemblyline.PortableResult,
			validationErr error,
		) error {
			finalizations++
			if validationErr != nil {
				return validationErr
			}
			if result.Candidate != body || strings.Contains(result.Candidate, signature) {
				return fmt.Errorf("accepted provider response is not the ordinary body: %q", result.Candidate)
			}
			return nil
		},
	}

	source, err := runDirectCodingTypeScriptFragmentWorker(
		runtime,
		"fixture-model",
		directCodingTypeScriptFragmentJob{
			block: assemblyline.SourceBlock{
				ID: "source.001", Signature: signature,
				Contract: "Return the sum of the two supplied values.",
				Role:     assemblyline.SourceBlockTaskImplementation,
			},
			dialect: "TypeScript",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := signature + " {\n" + body + "\n}"
	if source != want || strings.Count(source, signature) != 1 {
		t.Fatalf("code-owned source=%q; want %q", source, want)
	}
	if executions != 1 || corrections != 0 || finalizations != 1 {
		t.Fatalf(
			"worker calls execute=%d correct=%d finalize=%d; want 1,0,1",
			executions, corrections, finalizations,
		)
	}
}

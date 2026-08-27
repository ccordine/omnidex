package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestGoFragmentModificationFailsAfterOneInitialCandidate(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentModificationInput{
		Language: "go", Dialect: "Go 1.24", Signature: "func Value() int",
		CurrentDeclaration: "func Value() int { return 1 }",
		RequirementQuote:   "return two",
	}
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3, CorrectionModel: "forbidden-corrector",
		Execute: func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
			calls++
			if job.Kind != assemblyline.WorkFragmentModification || model != "coder" {
				t.Fatalf("initial call kind=%q model=%q", job.Kind, model)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: "func Value() string { return \"two\" }"}, nil
		},
	}
	_, err := runDirectCodingGoFragmentModificationWorker(runtime, "coder", directCodingGoModificationJob{
		Subject: "symbol-1", Input: input,
	})
	if err == nil || !strings.Contains(err.Error(), "changed its exact signature") || calls != 1 {
		t.Fatalf("error=%v calls=%d", err, calls)
	}
}

func TestGoFragmentModificationRejectsUnchangedCandidate(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentModificationInput{
		Language: "go", Dialect: "Go 1.24", Signature: "func Value() int",
		CurrentDeclaration: "func Value() int { return 1 }",
		RequirementQuote:   "return two",
	}
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			return assemblyline.PortableResult{JobID: job.ID, Candidate: input.CurrentDeclaration}, nil
		},
	}
	_, err := runDirectCodingGoFragmentModificationWorker(runtime, "coder", directCodingGoModificationJob{
		Subject: "symbol-1", Input: input,
	})
	if err == nil || !strings.Contains(err.Error(), "unchanged modification rejected") {
		t.Fatalf("unchanged candidate error=%v", err)
	}
}

func TestGoFragmentModificationFinalizesExactDeclarationProjection(t *testing.T) {
	t.Parallel()
	const raw = "```go\nfunc Value() int { return 2 }\n```"
	const declaration = "func Value() int { return 2 }"
	input := assemblyline.FragmentModificationInput{
		Language: "go", Dialect: "Go 1.24", Signature: "func Value() int",
		CurrentDeclaration: "func Value() int { return 1 }",
		RequirementQuote:   "return two",
	}
	finalized := false
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			return assemblyline.PortableResult{JobID: job.ID, Candidate: raw}, nil
		},
		Finalize: func(
			_ assemblyline.PortableJob,
			result assemblyline.PortableResult,
			validationErr error,
		) error {
			if validationErr != nil || result.Candidate != raw || result.Projection == nil ||
				result.Projection.Kind != assemblyline.PortableResultProjectionSourceDeclaration ||
				result.Projection.Source != declaration {
				t.Fatalf("finalized result=%+v validation=%v", result, validationErr)
			}
			finalized = true
			return nil
		},
	}
	got, err := runDirectCodingGoFragmentModificationWorker(
		runtime, "coder", directCodingGoModificationJob{Subject: "symbol-1", Input: input},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got != declaration || !finalized {
		t.Fatalf("source=%q finalized=%t", got, finalized)
	}
}

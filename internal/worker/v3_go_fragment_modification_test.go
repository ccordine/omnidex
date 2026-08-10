package worker

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestGoFragmentModificationUsesNarrowCorrectionEnvelope(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentModificationInput{
		Language: "go", Signature: "func Value() int",
		CurrentDeclaration: "func Value() int { return 1 }",
		RequirementQuote:   "return two",
	}
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 2, CorrectionModel: "corrector",
		Execute: func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
			calls++
			if calls == 1 {
				if job.Kind != assemblyline.WorkFragmentModification || model != "coder" {
					t.Fatalf("initial call kind=%q model=%q", job.Kind, model)
				}
				return assemblyline.PortableResult{JobID: job.ID, Candidate: "func Value() string { return \"two\" }"}, nil
			}
			if job.Kind != assemblyline.WorkFragmentCorrection || model != "corrector" {
				t.Fatalf("correction call kind=%q model=%q", job.Kind, model)
			}
			var correction assemblyline.FragmentCorrectionInput
			if err := json.Unmarshal(job.Payload, &correction); err != nil {
				t.Fatal(err)
			}
			encoded := string(job.Payload)
			if strings.Contains(encoded, input.RequirementQuote) {
				t.Fatalf("correction replayed the original requirement: %s", encoded)
			}
			if correction.Diagnostic == "" || correction.CurrentDeclaration == input.CurrentDeclaration {
				t.Fatalf("correction did not retain the rejected candidate and exact failure: %+v", correction)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: "func Value() int { return 2 }"}, nil
		},
	}
	got, err := runDirectCodingGoFragmentModificationWorker(runtime, "coder", directCodingGoModificationJob{
		Subject: "symbol-1", Input: input,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "return 2") || calls != 2 {
		t.Fatalf("result=%q calls=%d", got, calls)
	}
}

func TestGoFragmentModificationRejectsUnchangedCandidate(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentModificationInput{
		Language: "go", Signature: "func Value() int",
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

package worker

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestRepositoryGoVerificationCorrectionUsesNarrowEnvelope(t *testing.T) {
	t.Parallel()
	snapshot, analysis, contract, candidates := repositoryCorrectionFixture(t)
	first := existingRepositoryVerificationSymbol(t, analysis, "First")
	target := repositoryCorrectionTarget(t, contract, first.ID)
	current := candidates[first.ID]
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: maxTypedWorkerAttempts,
		Execute: func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
			calls++
			if job.Kind != assemblyline.WorkFragmentCorrection || model != "stable-corrector" {
				t.Fatalf("correction call kind=%q model=%q", job.Kind, model)
			}
			var input assemblyline.FragmentCorrectionInput
			if err := json.Unmarshal(job.Payload, &input); err != nil {
				t.Fatal(err)
			}
			payload := string(job.Payload)
			for _, forbidden := range []string{
				target.RequirementQuote, snapshot.Root, "first.go", "TestFirst", "func Second",
			} {
				if forbidden != "" && strings.Contains(payload, forbidden) {
					t.Fatalf("correction envelope leaked forbidden context %q: %s", forbidden, payload)
				}
			}
			if input.Signature != target.Signature || input.CurrentDeclaration != current ||
				input.Diagnostic != "got 11, want 1" || input.RequiredChange != repositoryGoVerificationRequiredChange {
				t.Fatalf("correction input=%+v", input)
			}
			return assemblyline.PortableResult{
				JobID: job.ID, Candidate: "func First() int { return 1 }",
			}, nil
		},
	}
	corrected, err := runRepositoryGoVerificationCorrection(
		runtime, "stable-corrector", target, current, "got 11, want 1",
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !strings.Contains(corrected, "return 1") {
		t.Fatalf("calls=%d corrected=%q", calls, corrected)
	}
}

func TestRepositoryGoVerificationCorrectionRejectsNoProgressAndPathBearingDiagnostic(t *testing.T) {
	t.Parallel()
	_, analysis, contract, candidates := repositoryCorrectionFixture(t)
	first := existingRepositoryVerificationSymbol(t, analysis, "First")
	target := repositoryCorrectionTarget(t, contract, first.ID)
	current := candidates[first.ID]
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: maxTypedWorkerAttempts,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			return assemblyline.PortableResult{JobID: job.ID, Candidate: current}, nil
		},
	}
	if _, err := runRepositoryGoVerificationCorrection(
		runtime, "stable-corrector", target, current, "got 11, want 1",
	); err == nil || !strings.Contains(err.Error(), "no progress") {
		t.Fatalf("unchanged correction error=%v", err)
	}
	if _, err := runRepositoryGoVerificationCorrection(
		runtime, "stable-corrector", target, current, "/workspace/first.go:3: wrong",
	); err == nil || !strings.Contains(err.Error(), "path-free") {
		t.Fatalf("path-bearing diagnostic error=%v", err)
	}
	if _, err := runRepositoryGoVerificationCorrection(
		runtime, "stable-corrector", target, current, `"first.go" has wrong value`,
	); err == nil || !strings.Contains(err.Error(), "path-free") {
		t.Fatalf("file-name-bearing diagnostic error=%v", err)
	}
}

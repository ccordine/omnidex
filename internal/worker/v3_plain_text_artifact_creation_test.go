package worker

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestPlainTextArtifactCreationUsesOnlyItsOwnWorkKind(t *testing.T) {
	t.Parallel()
	input := assemblyline.PlainTextArtifactCreationInput{
		RequirementQuote: "Create ARTIFACT_1 containing the complete note: Release ready.",
	}
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			calls++
			if job.Kind != assemblyline.WorkPlainTextArtifactCreation {
				t.Fatalf("plain-text relation dispatched work kind %q", job.Kind)
			}
			return assemblyline.PortableResult{
				JobID: job.ID, Candidate: string(assemblyline.OneNewCompletePlainTextArtifactRequired),
			}, nil
		},
	}
	decision, err := classifyPlainTextArtifactCreation(runtime, "semantic", input, nil)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || decision.Relation != assemblyline.OneNewCompletePlainTextArtifactRequired {
		t.Fatalf("calls=%d decision=%+v", calls, decision)
	}
}

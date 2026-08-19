package worker

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestExistingRepositoryRequirementsUseOneAggregateCall(t *testing.T) {
	t.Parallel()
	const request = "Add audit logging and CSV exports to the service."
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3,
		Execute: func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
			calls++
			if model != "qwen-stable" || job.Kind != assemblyline.WorkRepositoryRequirements {
				t.Fatalf("model=%q kind=%q", model, job.Kind)
			}
			candidate := assemblyline.RepositoryRequirementInterpretation{
				Schema:       assemblyline.RepositoryRequirementInterpretationSchemaV2,
				Requirements: []string{"CSV exports", "audit logging"},
			}
			raw, err := json.Marshal(candidate)
			return assemblyline.PortableResult{JobID: job.ID, Candidate: string(raw)}, err
		},
	}
	context, err := assemblyline.BootstrapApplicationContext(
		request, assemblyline.ApplicationWorkspaceExisting, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	quotes, err := interpretRepositoryRequirements(runtime, "qwen-stable", request, context, nil)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("semantic calls=%d", calls)
	}
	if !reflect.DeepEqual(quotes, []string{"CSV exports", "audit logging"}) {
		t.Fatalf("requirements=%q", quotes)
	}
}

func TestInvalidExistingRepositoryRequirementsFailWithoutCorrection(t *testing.T) {
	t.Parallel()
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			calls++
			candidate := assemblyline.RepositoryRequirementInterpretation{
				Schema:       "omnidex.repository-requirements.invalid",
				Requirements: []string{"invented change"},
			}
			raw, err := json.Marshal(candidate)
			return assemblyline.PortableResult{JobID: job.ID, Candidate: string(raw)}, err
		},
	}
	const request = "Add audit logging to the service."
	context, err := assemblyline.BootstrapApplicationContext(
		request, assemblyline.ApplicationWorkspaceExisting, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = interpretRepositoryRequirements(
		runtime, "qwen-stable", request, context, nil,
	)
	if err == nil {
		t.Fatal("ungrounded repository requirement succeeded")
	}
	if calls != 1 {
		t.Fatalf("invalid aggregate made %d semantic calls", calls)
	}
}

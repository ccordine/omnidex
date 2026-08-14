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
				Schema:        assemblyline.RepositoryRequirementInterpretationSchemaV1,
				FeatureQuotes: []string{"CSV exports", "audit logging"},
			}
			raw, err := json.Marshal(candidate)
			return assemblyline.PortableResult{JobID: job.ID, Candidate: string(raw)}, err
		},
	}
	quotes, err := interpretRepositoryRequirements(runtime, "qwen-stable", request, nil)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("semantic calls=%d", calls)
	}
	if !reflect.DeepEqual(quotes, []string{"audit logging", "CSV exports"}) {
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
				Schema:        assemblyline.RepositoryRequirementInterpretationSchemaV1,
				FeatureQuotes: []string{"invented change"},
			}
			raw, err := json.Marshal(candidate)
			return assemblyline.PortableResult{JobID: job.ID, Candidate: string(raw)}, err
		},
	}
	_, err := interpretRepositoryRequirements(
		runtime, "qwen-stable", "Add audit logging to the service.", nil,
	)
	if err == nil {
		t.Fatal("ungrounded repository requirement succeeded")
	}
	if calls != 1 {
		t.Fatalf("invalid aggregate made %d semantic calls", calls)
	}
}

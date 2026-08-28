package worker

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestExistingRepositoryRequirementsUseCoverageAndOneRawLeafCalls(t *testing.T) {
	t.Parallel()
	const request = "Add audit logging and CSV exports to the service."
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3,
		Execute: func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
			calls++
			if model != "qwen-stable" {
				t.Fatalf("model=%q kind=%q", model, job.Kind)
			}
			var input assemblyline.RepositoryRequirementLeafInput
			if err := json.Unmarshal(job.Payload, &input); err != nil {
				t.Fatal(err)
			}
			candidate := ""
			switch job.Kind {
			case assemblyline.WorkRepositoryRequirementCoverage:
				if len(input.AcceptedRequirements) < 2 {
					candidate = assemblyline.RepositoryRequirementRemains
				} else {
					candidate = assemblyline.RepositoryNoUncoveredRequirement
				}
			case assemblyline.WorkRepositoryRequirement:
				if len(input.AcceptedRequirements) == 0 {
					candidate = "audit logging"
				} else {
					candidate = "CSV exports"
				}
			default:
				t.Fatalf("unexpected work kind=%q", job.Kind)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
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
	if calls != 4 {
		t.Fatalf("semantic calls=%d", calls)
	}
	if !reflect.DeepEqual(quotes, []string{"audit logging", "CSV exports"}) {
		t.Fatalf("requirements=%q", quotes)
	}
}

func TestExistingRepositoryRequirementsExtractFirstLeafBeforeCoverage(t *testing.T) {
	t.Parallel()
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			calls++
			candidate := ""
			switch calls {
			case 1:
				if job.Kind != assemblyline.WorkRepositoryRequirement {
					t.Fatalf("first semantic work kind=%q", job.Kind)
				}
				candidate = "audit logging"
			case 2:
				if job.Kind != assemblyline.WorkRepositoryRequirementCoverage {
					t.Fatalf("second semantic work kind=%q", job.Kind)
				}
				candidate = assemblyline.RepositoryNoUncoveredRequirement
			default:
				t.Fatalf("unexpected semantic call %d kind=%q", calls, job.Kind)
			}
			return assemblyline.PortableResult{
				JobID: job.ID, Candidate: candidate,
			}, nil
		},
	}
	const request = "Add audit logging to the service."
	context, err := assemblyline.BootstrapApplicationContext(
		request, assemblyline.ApplicationWorkspaceExisting, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	requirements, err := interpretRepositoryRequirements(
		runtime, "qwen-stable", request, context, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !reflect.DeepEqual(requirements, []string{"audit logging"}) {
		t.Fatalf("calls=%d requirements=%q", calls, requirements)
	}
}

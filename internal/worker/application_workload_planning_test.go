package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestApplicationWorkloadPlansOneRawLeafAtATimeWithoutReview(t *testing.T) {
	t.Parallel()
	input := assemblyline.ApplicationWorkloadDraftInput{
		Surface:      assemblyline.ApplicationSurfaceBrowser,
		ProductQuote: "A browser counter",
		Requirements: []assemblyline.Requirement{
			{ID: "requirement_001", SourceQuote: "Display the current count."},
			{ID: "requirement_002", SourceQuote: "Increase the count when the increment control is activated."},
		},
	}
	calls := make(map[assemblyline.WorkKind]int)
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, modelName string) (assemblyline.PortableResult, error) {
			_, err := assemblyline.RenderPortableJob(job)
			if err != nil {
				return assemblyline.PortableResult{}, err
			}
			calls[job.Kind]++
			if modelName != "planner-model" {
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected planning model=%q", modelName)
			}
			var candidate string
			switch job.Kind {
			case assemblyline.WorkApplicationJobObjective:
				var input assemblyline.ApplicationJobSpecificationInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				candidate = "Implement " + input.FocusedRequirement.SourceQuote
			case assemblyline.WorkApplicationBehaviorCoverage:
				var input assemblyline.ApplicationJobBehaviorLeafInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if input.AcceptedBehaviors == nil {
					return assemblyline.PortableResult{}, fmt.Errorf("application behavior coverage received a nil accepted set")
				}
				if len(input.AcceptedBehaviors) == 0 {
					return assemblyline.PortableResult{}, fmt.Errorf("application behavior coverage received an empty accepted set")
				}
				candidate = assemblyline.ApplicationNoUncoveredBehavior
			case assemblyline.WorkApplicationBehavior:
				var input assemblyline.ApplicationJobBehaviorLeafInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if input.AcceptedBehaviors == nil {
					return assemblyline.PortableResult{}, fmt.Errorf("application behavior received a nil accepted set")
				}
				candidate = input.Authority.FocusedRequirement.SourceQuote
			case assemblyline.WorkApplicationCriterionCoverage:
				var input assemblyline.ApplicationJobCriterionLeafInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if input.AcceptedCriteria == nil {
					return assemblyline.PortableResult{}, fmt.Errorf("application criterion coverage received a nil accepted set")
				}
				if len(input.AcceptedCriteria) == 0 {
					return assemblyline.PortableResult{}, fmt.Errorf("application criterion coverage received an empty accepted set")
				}
				candidate = assemblyline.ApplicationNoUncoveredCriterion
			case assemblyline.WorkApplicationCriterion:
				var input assemblyline.ApplicationJobCriterionLeafInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if input.AcceptedCriteria == nil {
					return assemblyline.PortableResult{}, fmt.Errorf("application criterion received a nil accepted set")
				}
				candidate = input.Authority.FocusedRequirement.SourceQuote + " is observable in the browser."
			default:
				return assemblyline.PortableResult{}, fmt.Errorf(
					"unexpected planning call kind=%q", job.Kind,
				)
			}
			if strings.HasPrefix(strings.TrimSpace(candidate), "{") ||
				strings.HasPrefix(strings.TrimSpace(candidate), "[") {
				return assemblyline.PortableResult{}, fmt.Errorf("planning model returned structured output")
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
		},
	}
	workload, err := resolveDirectCodingApplicationWorkload(runtime, "planner-model", input)
	if err != nil {
		t.Fatal(err)
	}
	want := map[assemblyline.WorkKind]int{
		assemblyline.WorkApplicationJobObjective:      len(input.Requirements),
		assemblyline.WorkApplicationBehaviorCoverage:  len(input.Requirements),
		assemblyline.WorkApplicationBehavior:          len(input.Requirements),
		assemblyline.WorkApplicationCriterionCoverage: len(input.Requirements),
		assemblyline.WorkApplicationCriterion:         len(input.Requirements),
	}
	if len(calls) != len(want) {
		t.Fatalf("planning calls=%v want=%v", calls, want)
	}
	for kind, count := range want {
		if calls[kind] != count {
			t.Fatalf("planning calls[%s]=%d want=%d; calls=%v", kind, calls[kind], count, calls)
		}
	}
	if err := assemblyline.ValidateFrozenApplicationWorkload(input, workload); err != nil {
		t.Fatalf("frozen workload=%+v: %v", workload, err)
	}
}

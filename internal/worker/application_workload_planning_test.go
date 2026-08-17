package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestApplicationWorkloadPlansOneFocusedJobPerRequirementWithoutReview(t *testing.T) {
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
			calls[job.Kind]++
			if job.Kind != assemblyline.WorkApplicationJobSpecification || modelName != "planner-model" {
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected planning call kind=%q model=%q", job.Kind, modelName)
			}
			var authority assemblyline.ApplicationJobSpecificationInput
			if err := json.Unmarshal(job.Payload, &authority); err != nil {
				return assemblyline.PortableResult{}, err
			}
			candidate := assemblyline.ApplicationJobSpecification{
				Objective:          "Implement " + authority.FocusedRequirement.SourceQuote,
				RequiredBehaviors:  []string{authority.FocusedRequirement.SourceQuote},
				AcceptanceCriteria: []string{authority.FocusedRequirement.SourceQuote + " is observable in the browser."},
			}
			raw, err := json.Marshal(candidate)
			return assemblyline.PortableResult{JobID: job.ID, Candidate: string(raw)}, err
		},
	}
	workload, err := resolveDirectCodingApplicationWorkload(runtime, "planner-model", input)
	if err != nil {
		t.Fatal(err)
	}
	if calls[assemblyline.WorkApplicationJobSpecification] != len(input.Requirements) || len(calls) != 1 {
		t.Fatalf("planning calls=%v; one planner call per focused requirement is required", calls)
	}
	if err := assemblyline.ValidateFrozenApplicationWorkload(input, workload); err != nil {
		t.Fatalf("frozen workload=%+v: %v", workload, err)
	}
}

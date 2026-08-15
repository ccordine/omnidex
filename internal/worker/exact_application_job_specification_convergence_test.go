package worker

import (
	"context"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
)

func TestExactApplicationJobSpecificationConvergenceUsesProductionReviewLoop(t *testing.T) {
	t.Parallel()
	requirement := assemblyline.Requirement{ID: "requirement_001", SourceQuote: "filters inventory"}
	authority := assemblyline.ApplicationJobSpecificationInput{
		Surface: assemblyline.ApplicationSurfaceBrowser, ProductQuote: "inventory console",
		AcceptedRequirements: []assemblyline.Requirement{requirement}, FocusedRequirement: requirement,
	}
	job, err := assemblyline.NewApplicationJobSpecificationJob(authority)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	result, err := convergeExactApplicationJobSpecificationWithReplay(
		context.Background(), 47, 49, job, "planner", "reviewer",
		func(job assemblyline.PortableJob, model string, number int) (ExactStationReplay, error) {
			calls++
			var candidate string
			switch number {
			case 1:
				candidate = `{"objective":"Implement inventory filtering in the inventory console.","required_behaviors":["Users can filter visible inventory."],"acceptance_criteria":["Filtering changes the visible inventory."]}`
			case 2:
				if model != "reviewer" || job.Kind != assemblyline.WorkApplicationJobSpecificationReview {
					return ExactStationReplay{}, fmt.Errorf("call 2 model=%s kind=%s", model, job.Kind)
				}
				candidate = `{"decision":"repair","field":"acceptance_criteria"}`
			case 3:
				if model != "planner" || job.Kind != assemblyline.WorkApplicationJobSpecificationRepair {
					return ExactStationReplay{}, fmt.Errorf("call 3 model=%s kind=%s", model, job.Kind)
				}
				candidate = `{"acceptance_criteria":["Entering a filter shows matching inventory and excludes nonmatching inventory."]}`
			case 4:
				if model != "reviewer" || job.Kind != assemblyline.WorkApplicationJobSpecificationReview {
					return ExactStationReplay{}, fmt.Errorf("call 4 model=%s kind=%s", model, job.Kind)
				}
				candidate = `{"decision":"accept"}`
			default:
				return ExactStationReplay{}, fmt.Errorf("unexpected call %d", number)
			}
			return exactApplicationSpecificationTestReplay(job, model, candidate), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 4 || result.Terminal != "accepted" || len(result.Calls) != 4 ||
		result.SourceOpeningID != 47 || result.SourceGapOpeningID != 49 {
		t.Fatalf("convergence=%+v calls=%d", result, calls)
	}
	if result.Specification.AcceptanceCriteria[0] !=
		"Entering a filter shows matching inventory and excludes nonmatching inventory." {
		t.Fatalf("specification=%+v", result.Specification)
	}
}

func exactApplicationSpecificationTestReplay(
	job assemblyline.PortableJob,
	model string,
	candidate string,
) ExactStationReplay {
	return ExactStationReplay{
		Job: job, Model: model,
		Generation: llm.PreparedGeneration{Content: candidate},
		Artifact: ExactStationReplayArtifact{
			Kind: "exact_final_response", Source: candidate,
			SourceSHA256: replaySHA256(candidate), EndByte: len(candidate),
		},
	}
}

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
				candidate = `{"decision":"repair","field":"acceptance_criteria","finding":"The check does not distinguish matching inventory from nonmatching inventory.","finding_evidence":"Filtering changes the visible inventory."}`
			case 3:
				if model != "reviewer" || job.Kind != assemblyline.WorkApplicationJobSpecificationRepair {
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

func TestExactApplicationJobSpecificationConvergenceRestoresNoOpRepairOpening(t *testing.T) {
	t.Parallel()
	requirement := assemblyline.Requirement{
		ID: "requirement_001", SourceQuote: "Users can archive one completed inventory count.",
	}
	authority := assemblyline.ApplicationJobSpecificationInput{
		Surface: assemblyline.ApplicationSurfaceBrowser, ProductQuote: "inventory console",
		AcceptedRequirements: []assemblyline.Requirement{requirement}, FocusedRequirement: requirement,
	}
	retained := assemblyline.ApplicationJobSpecification{
		Objective: "Implement completed-count archival and report export.",
		RequiredBehaviors: []string{
			"Users archive one completed inventory count.",
		},
		AcceptanceCriteria: []string{
			"The archived count is no longer active.",
		},
	}
	reviewInput, err := assemblyline.NewApplicationJobSpecificationReviewInput(
		authority, retained, 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	reviewJob, err := assemblyline.NewApplicationJobSpecificationReviewJob(reviewInput)
	if err != nil {
		t.Fatal(err)
	}
	review, err := assemblyline.DecodeApplicationJobSpecificationReviewResult(
		reviewJob,
		`{"decision":"repair","field":"objective","finding":"The objective includes unrelated report export work.","finding_evidence":"Implement completed-count archival and report export."}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	repairInput, err := assemblyline.NewApplicationJobSpecificationRepairInput(
		authority, retained, review, 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	repairJob, err := assemblyline.NewApplicationJobSpecificationRepairJob(repairInput)
	if err != nil {
		t.Fatal(err)
	}

	result, err := convergeExactApplicationJobSpecificationWithReplay(
		context.Background(), 71, 73, repairJob, "planner", "reviewer",
		func(job assemblyline.PortableJob, model string, number int) (ExactStationReplay, error) {
			switch number {
			case 1:
				if model != "reviewer" || job.Kind != assemblyline.WorkApplicationJobSpecificationRepair {
					return ExactStationReplay{}, fmt.Errorf("call 1 model=%s kind=%s", model, job.Kind)
				}
				return exactApplicationSpecificationTestReplay(
					job, model,
					`{"objective":"Implement completed-count archival and report export."}`,
				), nil
			case 2:
				if model != "reviewer" || job.Kind != assemblyline.WorkResponseCorrection {
					return ExactStationReplay{}, fmt.Errorf("call 2 model=%s kind=%s", model, job.Kind)
				}
				return exactApplicationSpecificationTestReplay(
					job, model,
					`{"objective":"Implement archival of one completed inventory count."}`,
				), nil
			case 3:
				if model != "reviewer" || job.Kind != assemblyline.WorkApplicationJobSpecificationReview {
					return ExactStationReplay{}, fmt.Errorf("call 3 model=%s kind=%s", model, job.Kind)
				}
				return exactApplicationSpecificationTestReplay(
					job, model, `{"decision":"accept"}`,
				), nil
			default:
				return ExactStationReplay{}, fmt.Errorf("unexpected call %d", number)
			}
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Terminal != "accepted" || len(result.Calls) != 3 ||
		result.Specification.Objective != "Implement archival of one completed inventory count." {
		t.Fatalf("convergence=%+v", result)
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

package worker

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestApplicationWorkloadReviewUsesIndependentModelAndProgressesBeyondTenRepairs(t *testing.T) {
	t.Parallel()

	for _, fixture := range []struct {
		name        string
		requirement string
		product     string
	}{
		{name: "appointments", requirement: "schedules appointments", product: "appointment desk"},
		{name: "catalog", requirement: "organizes catalog entries", product: "catalog workspace"},
	} {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			assertApplicationWorkloadReviewProgressesThroughTwelveRepairs(
				t, fixture.requirement, fixture.product,
			)
		})
	}
}

func assertApplicationWorkloadReviewProgressesThroughTwelveRepairs(
	t *testing.T,
	requirement string,
	product string,
) {
	t.Helper()

	input := oneRequirementWorkloadInput(requirement, product)
	reviews, repairs := 0, 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
			switch job.Kind {
			case assemblyline.WorkApplicationJobSpecification:
				if model != "planner" {
					return assemblyline.PortableResult{}, fmt.Errorf("specification model=%q", model)
				}
				return workloadPortableCandidate(job, fmt.Sprintf(
					`{"objective":%q,"required_behaviors":[%q],"acceptance_criteria":[%q]}`,
					"Implement "+requirement+" in the "+product+".",
					"Users can "+requirement+".",
					"The result of "+requirement+" is visible.",
				)), nil
			case assemblyline.WorkApplicationJobSpecificationReview:
				if model != "llama-review" {
					return assemblyline.PortableResult{}, fmt.Errorf("review model=%q", model)
				}
				reviews++
				if reviews <= 12 {
					currentObjective := "Implement " + requirement + " in the " + product + "."
					if repairs > 0 {
						currentObjective = fmt.Sprintf(
							"Implement %s revision %d in the %s.", requirement, repairs, product,
						)
					}
					return workloadPortableCandidate(
						job, fmt.Sprintf(
							`{"decision":"repair","field":"objective","finding":"The objective does not state the focused local outcome.","finding_evidence":%q}`,
							currentObjective,
						),
					), nil
				}
				return workloadPortableCandidate(job, `{"decision":"accept"}`), nil
			case assemblyline.WorkApplicationJobSpecificationRepair:
				if model != "planner" {
					return assemblyline.PortableResult{}, fmt.Errorf("repair model=%q", model)
				}
				repairs++
				return workloadPortableCandidate(job, fmt.Sprintf(
					`{"objective":%q}`,
					fmt.Sprintf("Implement %s revision %d in the %s.", requirement, repairs, product),
				)), nil
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %s", job.Kind)
			}
		},
	}

	frozen, err := resolveDirectCodingApplicationWorkload(
		runtime, "planner", "llama-review", input,
	)
	if err != nil {
		t.Fatal(err)
	}
	if reviews != 13 || repairs != 12 {
		t.Fatalf("reviews=%d repairs=%d want 13/12", reviews, repairs)
	}
	if len(frozen.Tasks) != 1 ||
		frozen.Tasks[0].Objective != fmt.Sprintf(
			"Implement %s revision 12 in the %s.", requirement, product,
		) {
		t.Fatalf("frozen workload did not retain accepted progress: %+v", frozen)
	}
}

func TestApplicationWorkloadReviewStopsAnExactSpecificationStateCycle(t *testing.T) {
	t.Parallel()

	input := oneRequirementWorkloadInput("filters inventory", "inventory console")
	reviews, repairs := 0, 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			switch job.Kind {
			case assemblyline.WorkApplicationJobSpecification:
				return workloadPortableCandidate(job, `{
					"objective":"Implement inventory filtering in the inventory console.",
					"required_behaviors":["Users can apply an inventory filter."],
					"acceptance_criteria":["Applying the filter changes visible inventory."]
				}`), nil
			case assemblyline.WorkApplicationJobSpecificationReview:
				reviews++
				currentObjective := "Implement inventory filtering in the inventory console."
				if repairs == 1 {
					currentObjective = "Implement reviewed inventory filtering in the inventory console."
				}
				return workloadPortableCandidate(job, fmt.Sprintf(
					`{"decision":"repair","field":"objective","finding":"The objective does not state the focused local outcome.","finding_evidence":%q}`,
					currentObjective,
				)), nil
			case assemblyline.WorkApplicationJobSpecificationRepair:
				repairs++
				objective := "Implement reviewed inventory filtering in the inventory console."
				if repairs == 2 {
					objective = "Implement inventory filtering in the inventory console."
				}
				return workloadPortableCandidate(job, fmt.Sprintf(`{"objective":%q}`, objective)), nil
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %s", job.Kind)
			}
		},
	}

	_, err := resolveDirectCodingApplicationWorkload(runtime, "planner", "reviewer", input)
	if err == nil || !strings.Contains(err.Error(), "repeated specification state") {
		t.Fatalf("cycle error=%v", err)
	}
	if reviews != 2 || repairs != 2 {
		t.Fatalf("cycle calls reviews=%d repairs=%d want 2/2", reviews, repairs)
	}
}

func oneRequirementWorkloadInput(
	requirement string,
	product string,
) assemblyline.ApplicationWorkloadDraftInput {
	return applicationWorkloadInput(assemblyline.ApplicationSpecification{
		Surface: assemblyline.ApplicationSurfaceBrowser, ProductQuote: product,
		Requirements: []assemblyline.Requirement{
			{ID: "requirement_001", SourceQuote: requirement},
		},
	})
}

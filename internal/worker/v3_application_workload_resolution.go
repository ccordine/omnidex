package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

// resolveDirectCodingApplicationWorkload advances after code-owned validation.
// It does not invoke a ceremonial reviewer for each planned semantic leaf.
func resolveDirectCodingApplicationWorkload(
	runtime typedWorkerRuntime,
	plannerModel string,
	input assemblyline.ApplicationWorkloadDraftInput,
) (assemblyline.FrozenApplicationWorkload, error) {
	var zero assemblyline.FrozenApplicationWorkload
	if runtime.Context == nil || runtime.Execute == nil {
		return zero, fmt.Errorf("application workload resolver requires a portable execution runtime")
	}
	if runtime.MaxAttempts < 1 {
		return zero, fmt.Errorf("application workload runtime requires a positive attempt identity")
	}
	plannerModel = strings.TrimSpace(plannerModel)
	if plannerModel == "" {
		return zero, fmt.Errorf("application workload resolver requires a planner model")
	}
	specifications := make([]assemblyline.ApplicationJobSpecification, 0, len(input.Requirements))
	for index, requirement := range input.Requirements {
		authority := assemblyline.ApplicationJobSpecificationInput{
			Surface: input.Surface, ProductQuote: input.ProductQuote,
			AcceptedRequirements: append([]assemblyline.Requirement(nil), input.Requirements...),
			FocusedRequirement:   requirement,
		}
		specification, err := resolveDirectCodingApplicationJobSpecification(
			runtime, plannerModel, fmt.Sprintf("application_job_specification_%03d", index+1), authority,
		)
		if err != nil {
			return zero, err
		}
		specifications = append(specifications, specification)
	}
	draft, err := assemblyline.MaterializeApplicationWorkloadDraft(input, specifications)
	if err != nil {
		return zero, err
	}
	return assemblyline.FreezeApplicationWorkload(input, draft)
}

func resolveDirectCodingApplicationJobSpecification(
	runtime typedWorkerRuntime,
	plannerModel string,
	subject string,
	authority assemblyline.ApplicationJobSpecificationInput,
) (assemblyline.ApplicationJobSpecification, error) {
	var zero assemblyline.ApplicationJobSpecification
	job, err := assemblyline.NewApplicationJobSpecificationJob(authority)
	if err != nil {
		return zero, err
	}
	return runProgressiveApplicationJobSpecificationDraft(runtime, plannerModel, subject, job, authority)
}

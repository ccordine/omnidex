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
	objectiveJob, err := assemblyline.NewApplicationJobObjectiveJob(authority)
	if err != nil {
		return zero, err
	}
	objective, err := runDirectCodingSemanticLeafCall(
		runtime, plannerModel, subject+"_objective", objectiveJob, nil,
		func(raw string) (string, error) {
			return assemblyline.DecodeApplicationJobObjectiveLeaf(authority, raw)
		},
		func(value string) error {
			return assemblyline.ValidatePathFreeModelContextWithProvenance(
				"application job objective", runtime.PathProvenance, value,
			)
		},
	)
	if err != nil {
		return zero, err
	}

	behaviors := make([]string, 0, assemblyline.MaxApplicationRequiredBehaviorLeaves)
	for {
		leafInput := assemblyline.ApplicationJobBehaviorLeafInput{
			Authority: authority, Objective: objective,
			AcceptedBehaviors: append([]string{}, behaviors...),
		}
		if len(behaviors) > 0 {
			coverageJob, err := assemblyline.NewApplicationBehaviorCoverageJob(leafInput)
			if err != nil {
				return zero, err
			}
			coverage, err := runDirectCodingSemanticLeafCall(
				runtime, plannerModel, subject+"_behavior_coverage", coverageJob, nil,
				func(raw string) (string, error) {
					return assemblyline.DecodeApplicationBehaviorCoverageLeaf(leafInput, raw)
				},
				func(string) error { return nil },
			)
			if err != nil {
				return zero, err
			}
			if coverage == assemblyline.ApplicationNoUncoveredBehavior {
				break
			}
		}
		if len(behaviors) == assemblyline.MaxApplicationRequiredBehaviorLeaves {
			return zero, fmt.Errorf(
				"application behavior coverage remains incomplete at the code-owned %d-item bound",
				assemblyline.MaxApplicationRequiredBehaviorLeaves,
			)
		}
		behaviorJob, err := assemblyline.NewApplicationBehaviorJob(leafInput)
		if err != nil {
			return zero, err
		}
		behavior, err := runDirectCodingSemanticLeafCall(
			runtime, plannerModel, subject+"_behavior", behaviorJob, nil,
			func(raw string) (string, error) {
				return assemblyline.DecodeApplicationBehaviorLeaf(leafInput, raw)
			},
			func(value string) error {
				return assemblyline.ValidatePathFreeModelContextWithProvenance(
					"application behavior", runtime.PathProvenance, value,
				)
			},
		)
		if err != nil {
			return zero, err
		}
		behaviors = append(behaviors, behavior)
	}

	criteria := make([]string, 0, assemblyline.MaxApplicationAcceptanceCriterionLeaves)
	for {
		leafInput := assemblyline.ApplicationJobCriterionLeafInput{
			Authority: authority, Objective: objective,
			RequiredBehaviors: append([]string(nil), behaviors...),
			AcceptedCriteria:  append([]string{}, criteria...),
		}
		if len(criteria) > 0 {
			coverageJob, err := assemblyline.NewApplicationCriterionCoverageJob(leafInput)
			if err != nil {
				return zero, err
			}
			coverage, err := runDirectCodingSemanticLeafCall(
				runtime, plannerModel, subject+"_criterion_coverage", coverageJob, nil,
				func(raw string) (string, error) {
					return assemblyline.DecodeApplicationCriterionCoverageLeaf(leafInput, raw)
				},
				func(string) error { return nil },
			)
			if err != nil {
				return zero, err
			}
			if coverage == assemblyline.ApplicationNoUncoveredCriterion {
				break
			}
		}
		if len(criteria) == assemblyline.MaxApplicationAcceptanceCriterionLeaves {
			return zero, fmt.Errorf(
				"application criterion coverage remains incomplete at the code-owned %d-item bound",
				assemblyline.MaxApplicationAcceptanceCriterionLeaves,
			)
		}
		criterionJob, err := assemblyline.NewApplicationCriterionJob(leafInput)
		if err != nil {
			return zero, err
		}
		criterion, err := runDirectCodingSemanticLeafCall(
			runtime, plannerModel, subject+"_criterion", criterionJob, nil,
			func(raw string) (string, error) {
				return assemblyline.DecodeApplicationCriterionLeaf(leafInput, raw)
			},
			func(value string) error {
				return assemblyline.ValidatePathFreeModelContextWithProvenance(
					"application acceptance criterion", runtime.PathProvenance, value,
				)
			},
		)
		if err != nil {
			return zero, err
		}
		criteria = append(criteria, criterion)
	}

	specification := assemblyline.ApplicationJobSpecification{
		Objective: objective, RequiredBehaviors: behaviors,
		AcceptanceCriteria: criteria,
	}
	if err := assemblyline.ValidateApplicationJobSpecification(specification); err != nil {
		return zero, err
	}
	if err := specification.ValidatePathFree(runtime.PathProvenance); err != nil {
		return zero, err
	}
	return specification, nil
}

package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

// resolveDirectCodingTargetTree asks exactly one structural question after the
// semantic workload is frozen. The returned tree is data; all transitions are
// derived by code before any source worker runs.
func resolveDirectCodingTargetTree(
	runtime typedWorkerRuntime,
	modelName string,
	specification assemblyline.ApplicationSpecification,
	current []assemblyline.CurrentTargetArtifact,
) (assemblyline.TargetTree, error) {
	var zero assemblyline.TargetTree
	input := directCodingTargetTreeInput(specification, current)
	job, err := assemblyline.NewTargetTreeJob(input)
	if err != nil {
		return zero, err
	}
	candidate, err := runDirectCodingSemanticCall[assemblyline.TargetTreeCandidate](
		runtime, modelName, "application_target_tree", job, nil,
		func(value assemblyline.TargetTreeCandidate) error {
			_, validationErr := value.ValidateFor(input)
			return validationErr
		},
	)
	if err != nil {
		return zero, err
	}
	target, err := candidate.ValidateFor(input)
	if err != nil {
		return zero, fmt.Errorf("validate accepted target tree: %w", err)
	}
	if _, err := assemblyline.DiffTargetTree(input, target); err != nil {
		return zero, fmt.Errorf("derive target tree transitions: %w", err)
	}
	return target, nil
}

func directCodingTargetTreeInput(
	specification assemblyline.ApplicationSpecification,
	current []assemblyline.CurrentTargetArtifact,
) assemblyline.TargetTreeInput {
	requirements := make([]assemblyline.TargetTreeRequirement, len(specification.Requirements))
	for index, requirement := range specification.Requirements {
		requirements[index] = assemblyline.TargetTreeRequirement{ID: requirement.ID, Statement: requirement.SourceQuote}
	}
	inventory := make([]assemblyline.CurrentTargetArtifact, len(current))
	copy(inventory, current)
	return assemblyline.TargetTreeInput{
		Objective: specification.ProductQuote, Requirements: requirements, Current: inventory,
	}
}

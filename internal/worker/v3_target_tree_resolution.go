package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

// resolveDirectCodingTargetTree asks exactly one structural question after the
// semantic workload is frozen. The returned tree is data; all transitions are
// derived by code before any source worker runs.
func resolveDirectCodingTargetTree(
	runtime typedWorkerRuntime,
	plannerModel string,
	correctionModel string,
	specification assemblyline.ApplicationSpecification,
	current []assemblyline.CurrentTargetArtifact,
) (assemblyline.TargetTree, error) {
	var zero assemblyline.TargetTree
	input := directCodingTargetTreeInput(specification, current)
	target, err := runDirectCodingTargetTreeCall(runtime, plannerModel, correctionModel, input)
	if err != nil {
		return zero, err
	}
	if _, err := assemblyline.DiffTargetTree(input, target); err != nil {
		return zero, fmt.Errorf("derive target tree transitions: %w", err)
	}
	return target, nil
}

func runDirectCodingTargetTreeCall(
	runtime typedWorkerRuntime,
	plannerModel string,
	correctionModel string,
	input assemblyline.TargetTreeInput,
) (assemblyline.TargetTree, error) {
	var zero assemblyline.TargetTree
	if runtime.Context == nil || runtime.Execute == nil {
		return zero, fmt.Errorf("target tree requires a portable execution runtime")
	}
	if runtime.MaxAttempts < 1 || runtime.MaxAttempts > maxTypedWorkerAttempts {
		return zero, fmt.Errorf("target tree attempts must be between 1 and %d", maxTypedWorkerAttempts)
	}
	plannerModel = strings.TrimSpace(plannerModel)
	correctionModel = strings.TrimSpace(correctionModel)
	if plannerModel == "" || correctionModel == "" {
		return zero, fmt.Errorf("target tree requires configured planner and correction models")
	}
	var lastCandidate string
	var lastFailure error
	seen := make(map[string]struct{})
	for attempt := 1; attempt <= runtime.MaxAttempts; attempt++ {
		if err := runtime.Context.Err(); err != nil {
			return zero, fmt.Errorf("target tree authority ended: %w", err)
		}
		attemptInput := input
		modelName := plannerModel
		if lastFailure != nil {
			attemptInput.Correction = &assemblyline.TargetTreeCorrection{
				CandidateJSON: lastCandidate,
				Failure:       trimForBudget(lastFailure.Error(), 1200),
			}
			modelName = correctionModel
		}
		job, err := assemblyline.NewTargetTreeJob(attemptInput)
		if err != nil {
			return zero, err
		}
		result, err := runtime.Execute(job, modelName)
		if err != nil {
			return zero, fmt.Errorf("target tree inference: %w", err)
		}
		if err := result.ValidateFor(job); err != nil {
			return zero, finalizeTypedWorkerResult(runtime, job, result, err)
		}
		candidate := strings.TrimSpace(result.Candidate)
		if _, duplicate := seen[candidate]; duplicate {
			err := fmt.Errorf("target tree replacement made no semantic progress")
			return zero, finalizeTypedWorkerResult(runtime, job, result, err)
		}
		seen[candidate] = struct{}{}
		target, validationErr := assemblyline.DecodeTargetTreeCandidate(input, candidate)
		if validationErr == nil {
			if err := finalizeTypedWorkerResult(runtime, job, result, nil); err != nil {
				return zero, err
			}
			return target, nil
		}
		if err := persistTargetTreeRejection(runtime, job, result, validationErr); err != nil {
			return zero, err
		}
		lastCandidate = candidate
		lastFailure = validationErr
	}
	return zero, fmt.Errorf("target tree candidate failed %d bounded replacement attempts: %w", runtime.MaxAttempts, lastFailure)
}

func persistTargetTreeRejection(
	runtime typedWorkerRuntime,
	job assemblyline.PortableJob,
	result assemblyline.PortableResult,
	validationErr error,
) error {
	if runtime.Finalize == nil {
		return nil
	}
	if err := runtime.Finalize(job, result, validationErr); err != nil {
		return fmt.Errorf("persist target tree rejection: %w", err)
	}
	return nil
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

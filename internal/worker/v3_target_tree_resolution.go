package worker

import (
	"fmt"
	"sort"
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
	workload assemblyline.FrozenApplicationWorkload,
	stack directCodingProjectStack,
	existingPaths []string,
	existingDirs []string,
) (assemblyline.TargetTree, assemblyline.ApplicationFileCoveragePlan, error) {
	var zeroTree assemblyline.TargetTree
	var zeroCoverage assemblyline.ApplicationFileCoveragePlan
	if err := assemblyline.ValidateFrozenApplicationWorkload(applicationWorkloadInput(specification), workload); err != nil {
		return zeroTree, zeroCoverage, err
	}
	union := make(map[string]struct{})
	taskPaths := make(map[string][]string, len(workload.Tasks))
	for taskIndex, task := range workload.Tasks {
		currentPaths := directCodingTargetTreeCurrentPaths(existingPaths, union)
		input, err := directCodingFocusedTargetTreeInput(
			specification, task, stack, currentPaths, existingDirs,
		)
		if err != nil {
			return zeroTree, zeroCoverage, err
		}
		var focused assemblyline.TargetTree
		if stack.ProjectFocusedTargetTree != nil {
			focused, err = stack.ProjectFocusedTargetTree(taskIndex+1, currentPaths)
			if err == nil {
				err = validateDirectCodingTargetTreeAdapters(stack, focused)
			}
			if err == nil {
				err = validateDirectCodingFocusedTargetTreeOwnership(stack, focused, union)
			}
		} else {
			focused, err = runDirectCodingTargetTreeCall(
				runtime, plannerModel, correctionModel, input, stack, union,
			)
		}
		if err != nil {
			return zeroTree, zeroCoverage, fmt.Errorf("resolve focused target tree for %s: %w", task.ID, err)
		}
		if _, err := assemblyline.DiffTargetTree(input, focused); err != nil {
			return zeroTree, zeroCoverage, fmt.Errorf("derive focused target tree transitions for %s: %w", task.ID, err)
		}
		taskPaths[task.ID] = append([]string(nil), focused.Paths...)
		for _, artifactPath := range focused.Paths {
			union[artifactPath] = struct{}{}
		}
	}
	paths := make([]string, 0, len(union))
	for artifactPath := range union {
		paths = append(paths, artifactPath)
	}
	sort.Strings(paths)
	target := assemblyline.TargetTree{StackID: stack.ID, Paths: paths}
	coverage, err := buildDirectCodingApplicationFileCoveragePlan(stack, workload, target, taskPaths)
	if err != nil {
		return zeroTree, zeroCoverage, err
	}
	return target, coverage, nil
}

func runDirectCodingTargetTreeCall(
	runtime typedWorkerRuntime,
	plannerModel string,
	correctionModel string,
	input assemblyline.TargetTreeInput,
	stack directCodingProjectStack,
	reservedPaths map[string]struct{},
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
			validationErr = validateDirectCodingTargetTreeAdapters(stack, target)
		}
		if validationErr == nil {
			validationErr = validateDirectCodingFocusedTargetTreeOwnership(stack, target, reservedPaths)
		}
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

func directCodingTargetTreeCurrentPaths(
	existingPaths []string,
	reservedPaths map[string]struct{},
) []string {
	current := make(map[string]struct{}, len(existingPaths)+len(reservedPaths))
	for _, artifactPath := range existingPaths {
		current[artifactPath] = struct{}{}
	}
	for artifactPath := range reservedPaths {
		current[artifactPath] = struct{}{}
	}
	paths := make([]string, 0, len(current))
	for artifactPath := range current {
		paths = append(paths, artifactPath)
	}
	sort.Strings(paths)
	return paths
}

func validateDirectCodingFocusedTargetTreeOwnership(
	stack directCodingProjectStack,
	target assemblyline.TargetTree,
	reservedPaths map[string]struct{},
) error {
	if !stack.ExclusiveTaskPaths {
		return nil
	}
	for _, artifactPath := range target.Paths {
		if _, reserved := reservedPaths[artifactPath]; reserved {
			return fmt.Errorf(
				"focused target-tree path %q is already owned by a previously accepted task",
				artifactPath,
			)
		}
	}
	return nil
}

// validateDirectCodingTargetTreeAdapters keeps a path-only tree inside the
// selected code-owned project stack before any per-file semantic work starts.
// A model never chooses adapters; an incompatible path is an invalid tree
// declaration and is corrected at the tree boundary.
func validateDirectCodingTargetTreeAdapters(stack directCodingProjectStack, target assemblyline.TargetTree) error {
	for _, filePath := range target.Paths {
		if _, _, err := directCodingArtifactAdapterForTreePath(stack, filePath); err != nil {
			return err
		}
	}
	if stack.ValidateTargetTree == nil {
		return fmt.Errorf("project stack %s has no target-tree validator", stack.ID)
	}
	return stack.ValidateTargetTree(target)
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
	stack directCodingProjectStack,
	existingPaths []string,
	existingDirs []string,
) (assemblyline.TargetTreeInput, error) {
	if !stack.SupportsSurface(specification.Surface) {
		return assemblyline.TargetTreeInput{}, fmt.Errorf(
			"selected project stack %s supports surfaces %s, not %s",
			stack.ID, directCodingProjectStackSurfaceSummary(stack.SupportedSurfaces), specification.Surface,
		)
	}
	technicalContext, err := directCodingTreeTechnicalContext(stack)
	if err != nil {
		return assemblyline.TargetTreeInput{}, err
	}
	paths := make([]string, len(existingPaths))
	copy(paths, existingPaths)
	directories := make([]string, len(existingDirs))
	copy(directories, existingDirs)
	return assemblyline.TargetTreeInput{
		Objective:        specification.ProductQuote,
		TechnicalContext: technicalContext,
		ExistingPaths:    paths,
		ExistingDirs:     directories,
	}, nil
}

func directCodingFocusedTargetTreeInput(
	specification assemblyline.ApplicationSpecification,
	task assemblyline.FrozenApplicationTask,
	stack directCodingProjectStack,
	existingPaths []string,
	existingDirs []string,
) (assemblyline.TargetTreeInput, error) {
	input, err := directCodingTargetTreeInput(specification, stack, existingPaths, existingDirs)
	if err != nil {
		return assemblyline.TargetTreeInput{}, err
	}
	input.Objective = strings.Join([]string{
		"Product context: " + specification.ProductQuote,
		"Accepted behavior: " + task.RequirementQuote,
		"Structural objective: " + task.Objective,
	}, "\n")
	if err := input.Validate(); err != nil {
		return assemblyline.TargetTreeInput{}, err
	}
	return input, nil
}

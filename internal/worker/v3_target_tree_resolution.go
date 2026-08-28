package worker

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

// resolveDirectCodingTargetTree resolves one complete workload tree. An
// inferred stack receives every accepted goal in one call. Mechanical stacks
// retain their code-owned per-task allocation and make no inference calls.
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
	input, err := directCodingTargetTreeInput(
		specification, workload, stack, existingPaths, existingDirs,
	)
	if err != nil {
		return zeroTree, zeroCoverage, err
	}
	taskPaths := make(map[string][]string, len(workload.Tasks))
	var target assemblyline.TargetTree
	if stack.ProjectFocusedTargetTree == nil {
		target, err = runDirectCodingTargetTreeCall(
			runtime, plannerModel, correctionModel, input, stack,
		)
		if err == nil {
			for _, task := range workload.Tasks {
				taskPaths[task.ID] = append([]string(nil), target.Paths...)
			}
		}
	} else {
		union := make(map[string]struct{})
		for taskIndex, task := range workload.Tasks {
			var focused assemblyline.TargetTree
			focused, err = stack.ProjectFocusedTargetTree(
				taskIndex+1, directCodingTargetTreeOccupiedPaths(input, union),
			)
			if err == nil {
				err = assemblyline.ValidateTargetTreeReservedPaths(input.ReservedPaths, focused)
			}
			if err == nil {
				err = validateDirectCodingFocusedTargetTree(stack, focused)
			}
			if err != nil {
				return zeroTree, zeroCoverage, fmt.Errorf(
					"project target-tree pair for %s: %w", task.ID, err,
				)
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
		target = assemblyline.TargetTree{Paths: paths}
	}
	if err != nil {
		return zeroTree, zeroCoverage, fmt.Errorf("resolve complete target tree: %w", err)
	}
	target.StackID = stack.ID
	if err := validateDirectCodingTargetTreeUnion(stack, target); err != nil {
		return zeroTree, zeroCoverage, err
	}
	if _, err := assemblyline.DiffTargetTree(input, target, input.ExistingPaths); err != nil {
		return zeroTree, zeroCoverage, fmt.Errorf("derive target tree transitions: %w", err)
	}
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
) (assemblyline.TargetTree, error) {
	return runDirectCodingTargetTreeCallWithValidator(
		runtime, plannerModel, correctionModel, input,
		func(target assemblyline.TargetTree) error {
			return validateDirectCodingFocusedTargetTree(stack, target)
		},
	)
}

type directCodingTargetTreeCandidateValidator func(assemblyline.TargetTree) error

func runDirectCodingTargetTreeCallWithValidator(
	runtime typedWorkerRuntime,
	plannerModel string,
	correctionModel string,
	input assemblyline.TargetTreeInput,
	validateCandidate directCodingTargetTreeCandidateValidator,
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
	if validateCandidate == nil {
		return zero, fmt.Errorf("target tree requires one code-owned candidate validator")
	}
	var lastSafeCandidate string
	var lastCorrectionFailure string
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
				CandidateTree: lastSafeCandidate,
				Failure:       lastCorrectionFailure,
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
		candidate := result.Candidate
		if _, duplicate := seen[candidate]; duplicate {
			err := fmt.Errorf("target tree replacement made no semantic progress")
			return zero, finalizeTypedWorkerResult(runtime, job, result, err)
		}
		seen[candidate] = struct{}{}
		target, validationErr := assemblyline.DecodeTargetTreeCandidate(input, candidate)
		if validationErr == nil {
			validationErr = validateCandidate(target)
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
		lastSafeCandidate = ""
		lastCorrectionFailure = "The response violates the exact raw target-tree grammar."
		if parsed, syntaxErr := assemblyline.ParseTargetTree(candidate); syntaxErr == nil {
			canonical, renderErr := assemblyline.RenderTargetTree(parsed.Paths)
			if renderErr != nil {
				return zero, renderErr
			}
			lastSafeCandidate = canonical
			lastCorrectionFailure = trimForBudget(validationErr.Error(), 1200)
		}
		lastFailure = validationErr
	}
	return zero, fmt.Errorf("target tree candidate failed %d bounded replacement attempts: %w", runtime.MaxAttempts, lastFailure)
}

func directCodingTargetTreeOccupiedPaths(
	input assemblyline.TargetTreeInput,
	accepted map[string]struct{},
) []string {
	occupied := make(
		map[string]struct{},
		len(input.ExistingPaths)+len(input.ReservedPaths)+len(input.ExistingDirs)+len(accepted),
	)
	for _, paths := range [][]string{
		input.ExistingPaths, input.ReservedPaths, input.ExistingDirs,
	} {
		for _, artifactPath := range paths {
			occupied[artifactPath] = struct{}{}
		}
	}
	for artifactPath := range accepted {
		occupied[artifactPath] = struct{}{}
	}
	result := make([]string, 0, len(occupied))
	for artifactPath := range occupied {
		result = append(result, artifactPath)
	}
	sort.Strings(result)
	return result
}

// validateDirectCodingFocusedTargetTree applies constraints that belong to one
// frozen task's focused result. A model-resolved invalid tree may be corrected
// at this boundary; a mechanically projected invalid tree fails terminally.
func validateDirectCodingFocusedTargetTree(
	stack directCodingProjectStack,
	target assemblyline.TargetTree,
) error {
	if err := assemblyline.ValidateTargetTreeConstraints(stack.TargetTreeConstraints, target); err != nil {
		return err
	}
	if err := validateDirectCodingTargetTreeUnion(stack, target); err != nil {
		return err
	}
	if stack.ValidateTargetTree == nil {
		return fmt.Errorf("project stack %s has no target-tree validator", stack.ID)
	}
	return stack.ValidateTargetTree(target)
}

// validateDirectCodingTargetTreeUnion applies only invariants that remain true
// after focused task trees are unioned. Per-task cardinality and pair grammar
// are already proven before retention and must not be reapplied to the union.
func validateDirectCodingTargetTreeUnion(
	stack directCodingProjectStack,
	target assemblyline.TargetTree,
) error {
	if len(target.Paths) == 0 {
		return fmt.Errorf("project stack %s requires at least one target-tree path", stack.ID)
	}
	if err := assemblyline.ValidateTargetTreeReservedPaths(stack.TargetTreeReservedPaths, target); err != nil {
		return err
	}
	for _, filePath := range target.Paths {
		if _, _, err := directCodingArtifactAdapterForTreePath(stack, filePath); err != nil {
			return err
		}
	}
	return nil
}

func validateDirectCodingCoveredFocusedTargetTrees(
	stack directCodingProjectStack,
	workload assemblyline.FrozenApplicationWorkload,
	coverage assemblyline.ApplicationFileCoveragePlan,
) error {
	for _, task := range workload.Tasks {
		files, err := coverage.FilesForTask(task.ID)
		if err != nil {
			return err
		}
		paths := make([]string, len(files))
		for index, file := range files {
			paths[index] = file.Path
		}
		if err := validateDirectCodingFocusedTargetTree(
			stack, assemblyline.TargetTree{Paths: paths},
		); err != nil {
			return fmt.Errorf("focused task %s: %w", task.ID, err)
		}
	}
	return nil
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

package worker

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

// resolveDirectCodingTargetTree resolves one focused structural result for
// each frozen task. It invokes the target-tree station only when the selected
// stack leaves a genuine naming question unresolved. Every returned or
// projected tree is data; code derives all transitions before source work.
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
		earlierPaths := directCodingTargetTreeEarlierPaths(union)
		input, err := directCodingFocusedTargetTreeInput(
			specification, task, stack, existingPaths, earlierPaths, existingDirs,
		)
		if err != nil {
			return zeroTree, zeroCoverage, err
		}
		var focused assemblyline.TargetTree
		if stack.ProjectFocusedTargetTree != nil {
			focused, err = stack.ProjectFocusedTargetTree(
				taskIndex+1, directCodingTargetTreeOccupiedPaths(input),
			)
			if err == nil {
				err = assemblyline.ValidateTargetTreeReservedPaths(input.ReservedPaths, focused)
			}
			if err == nil {
				err = validateDirectCodingFocusedTargetTree(stack, focused)
			}
		} else {
			focused, err = runDirectCodingTargetTreeCall(
				runtime, plannerModel, correctionModel, input, stack,
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
		lastCandidate = candidate
		lastFailure = validationErr
	}
	return zero, fmt.Errorf("target tree candidate failed %d bounded replacement attempts: %w", runtime.MaxAttempts, lastFailure)
}

func directCodingTargetTreeEarlierPaths(accepted map[string]struct{}) []string {
	paths := make([]string, 0, len(accepted))
	for artifactPath := range accepted {
		paths = append(paths, artifactPath)
	}
	sort.Strings(paths)
	return paths
}

func directCodingTargetTreeOccupiedPaths(input assemblyline.TargetTreeInput) []string {
	occupied := make(
		map[string]struct{},
		len(input.ExistingPaths)+len(input.ReusablePaths)+len(input.ReservedPaths)+len(input.ExistingDirs),
	)
	for _, paths := range [][]string{
		input.ExistingPaths, input.ReusablePaths, input.ReservedPaths, input.ExistingDirs,
	} {
		for _, artifactPath := range paths {
			occupied[artifactPath] = struct{}{}
		}
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

func directCodingTargetTreeInput(
	specification assemblyline.ApplicationSpecification,
	stack directCodingProjectStack,
	existingPaths []string,
	earlierPaths []string,
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
	reusable := make([]string, 0, len(earlierPaths))
	reserved := make(
		[]string, 0, len(stack.TargetTreeReservedPaths)+len(earlierPaths),
	)
	reserved = append(reserved, stack.TargetTreeReservedPaths...)
	if stack.ExclusiveTaskPaths {
		reserved = append(reserved, earlierPaths...)
		sort.Strings(reserved)
	} else {
		reusable = append(reusable, earlierPaths...)
	}
	directories := make([]string, len(existingDirs))
	copy(directories, existingDirs)
	return assemblyline.TargetTreeInput{
		Objective:        specification.ProductQuote,
		TechnicalContext: technicalContext,
		Constraints:      stack.TargetTreeConstraints,
		ExistingPaths:    paths,
		ReusablePaths:    reusable,
		ReservedPaths:    reserved,
		ExistingDirs:     directories,
	}, nil
}

func directCodingFocusedTargetTreeInput(
	specification assemblyline.ApplicationSpecification,
	task assemblyline.FrozenApplicationTask,
	stack directCodingProjectStack,
	existingPaths []string,
	earlierPaths []string,
	existingDirs []string,
) (assemblyline.TargetTreeInput, error) {
	input, err := directCodingTargetTreeInput(
		specification, stack, existingPaths, earlierPaths, existingDirs,
	)
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

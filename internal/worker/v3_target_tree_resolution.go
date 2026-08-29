package worker

import (
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/assemblyline"
)

// resolveDirectCodingTargetTree resolves one complete workload tree. A
// complete mechanical stack allocates one tree for the whole workload, a
// focused mechanical stack allocates one tree per task, and only a stack with
// neither exact projector may receive one bounded semantic tree call.
func resolveDirectCodingTargetTree(
	runtime typedWorkerRuntime,
	initialModel string,
	replacementModel string,
	request string,
	specification assemblyline.ApplicationSpecification,
	workload assemblyline.FrozenApplicationWorkload,
	stack directCodingProjectStack,
	existingPaths []string,
	existingDirs []string,
) (assemblyline.TargetTree, assemblyline.ApplicationFileCoveragePlan, error) {
	var zeroTree assemblyline.TargetTree
	var zeroCoverage assemblyline.ApplicationFileCoveragePlan
	if err := assemblyline.ValidateFrozenApplicationWorkloadFor(specification, workload); err != nil {
		return zeroTree, zeroCoverage, err
	}
	input, err := directCodingTargetTreeInput(
		request, specification, workload, stack, existingPaths, existingDirs,
	)
	if err != nil {
		return zeroTree, zeroCoverage, err
	}
	taskPaths := make(map[string][]string, len(workload.Tasks))
	var target assemblyline.TargetTree
	switch {
	case stack.ProjectCompleteTargetTree != nil:
		if len(input.ExistingPaths) != 0 {
			target = assemblyline.TargetTree{Paths: append([]string(nil), input.ExistingPaths...)}
		} else {
			target, err = stack.ProjectCompleteTargetTree(
				directCodingTargetTreeOccupationFor(input, existingPaths, map[string]struct{}{}),
			)
		}
		if err == nil {
			err = assemblyline.ValidateTargetTreeReservedPaths(input.ReservedPaths, target)
		}
		if err == nil {
			err = validateDirectCodingFocusedTargetTree(stack, target)
		}
		if err != nil {
			return zeroTree, zeroCoverage, fmt.Errorf(
				"project complete target tree: %w", err,
			)
		}
		for _, task := range workload.Tasks {
			taskPaths[task.ID] = append([]string(nil), target.Paths...)
		}
	case stack.ProjectFocusedTargetTree == nil:
		runtime.MaxAttempts = maxDirectCodingTargetTreeCalls
		target, err = runDirectCodingTargetTreeCall(
			runtime, initialModel, replacementModel, input, existingPaths, stack,
		)
		if err == nil {
			for _, task := range workload.Tasks {
				taskPaths[task.ID] = append([]string(nil), target.Paths...)
			}
		}
	default:
		union := make(map[string]struct{})
		for taskIndex, task := range workload.Tasks {
			var focused assemblyline.TargetTree
			focused, err = stack.ProjectFocusedTargetTree(
				taskIndex+1, directCodingTargetTreeOccupationFor(input, existingPaths, union),
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
	if err := validateDirectCodingTargetTreePathClosure(input, existingPaths, target); err != nil {
		return zeroTree, zeroCoverage, err
	}
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

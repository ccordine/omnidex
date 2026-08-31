package worker

import (
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/assemblyline"
)

// resolveDirectCodingTargetTree resolves one complete workload tree from the
// selected stack's code-owned projection.
func resolveDirectCodingTargetTree(
	specification assemblyline.ApplicationSpecification,
	workload assemblyline.FrozenApplicationWorkload,
	stack directCodingProjectStack,
) (assemblyline.TargetTree, assemblyline.ApplicationFileCoveragePlan, error) {
	var zeroTree assemblyline.TargetTree
	var zeroCoverage assemblyline.ApplicationFileCoveragePlan
	taskPaths := make(map[string][]string, len(workload.Tasks))
	var target assemblyline.TargetTree
	var err error
	switch {
	case stack.ProjectCompleteTargetTree != nil:
		target, err = stack.ProjectCompleteTargetTree(
			directCodingTargetTreeOccupationFor(stack, map[string]struct{}{}),
		)
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
	case stack.ProjectFocusedTargetTree != nil:
		union := make(map[string]struct{})
		for taskIndex, task := range workload.Tasks {
			var focused assemblyline.TargetTree
			focused, err = stack.ProjectFocusedTargetTree(
				taskIndex+1, directCodingTargetTreeOccupationFor(stack, union),
			)
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
	default:
		return zeroTree, zeroCoverage, fmt.Errorf(
			"project stack %s has no code-owned target-tree projection", stack.ID,
		)
	}
	if err != nil {
		return zeroTree, zeroCoverage, fmt.Errorf("resolve complete target tree: %w", err)
	}
	if err := validateDirectCodingTargetTreePathClosure(stack, target); err != nil {
		return zeroTree, zeroCoverage, err
	}
	if err := validateDirectCodingTargetTreeUnion(stack, target); err != nil {
		return zeroTree, zeroCoverage, err
	}
	coverage, err := buildDirectCodingApplicationFileCoveragePlan(stack, workload, target, taskPaths)
	if err != nil {
		return zeroTree, zeroCoverage, err
	}
	return target, coverage, nil
}

// validateDirectCodingFocusedTargetTree applies constraints at the projected
// tree's first structural consumer.
func validateDirectCodingFocusedTargetTree(
	stack directCodingProjectStack,
	target assemblyline.TargetTree,
) error {
	return assemblyline.ValidateTargetTreeConstraints(stack.TargetTreeConstraints, target)
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

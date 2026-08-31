package worker

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingTargetTreeOccupation struct {
	FilePaths []string
}

func directCodingTargetTreeOccupationFor(
	input assemblyline.TargetTreeInput,
	accepted map[string]struct{},
) directCodingTargetTreeOccupation {
	files := make(
		map[string]struct{}, len(input.ReservedPaths)+len(accepted),
	)
	for _, artifactPath := range input.ReservedPaths {
		files[artifactPath] = struct{}{}
	}
	for artifactPath := range accepted {
		files[artifactPath] = struct{}{}
	}
	filePaths := make([]string, 0, len(files))
	for artifactPath := range files {
		filePaths = append(filePaths, artifactPath)
	}
	sort.Strings(filePaths)
	return directCodingTargetTreeOccupation{FilePaths: filePaths}
}

// validateDirectCodingTargetTreePathClosure proves only the tree's own
// reserved-path invariants. Existing filesystem state belongs to the
// reconciler, which consumes the complete desired state later.
func validateDirectCodingTargetTreePathClosure(
	input assemblyline.TargetTreeInput,
	target assemblyline.TargetTree,
) error {
	for _, candidate := range target.Paths {
		for _, occupied := range input.ReservedPaths {
			if directCodingTargetTreeFileHierarchyConflict(candidate, occupied) {
				return fmt.Errorf(
					"target-tree file %q crosses reserved file boundary %q", candidate, occupied,
				)
			}
		}
	}
	return nil
}

func directCodingTargetTreeFileHierarchyConflict(left, right string) bool {
	return left == right || strings.HasPrefix(left, right+"/") ||
		strings.HasPrefix(right, left+"/")
}

func directCodingTargetTreePathsAvailable(
	paths []string,
	occupation directCodingTargetTreeOccupation,
) (bool, error) {
	if len(paths) == 0 {
		return false, fmt.Errorf("mechanical target-tree grammar returned no paths")
	}
	for index, candidate := range paths {
		normalized, err := normalizeDirectCodingPath(candidate)
		if err != nil {
			return false, fmt.Errorf(
				"mechanical target-tree path %d is not normalized: %w", index, err,
			)
		}
		if normalized != candidate {
			return false, fmt.Errorf(
				"mechanical target-tree path %d changed during normalization", index,
			)
		}
		for otherIndex, other := range paths {
			if otherIndex == index {
				continue
			}
			if directCodingTargetTreeFileHierarchyConflict(candidate, other) {
				return false, fmt.Errorf("mechanical target-tree paths cross a file hierarchy boundary")
			}
		}
		for _, occupied := range occupation.FilePaths {
			if directCodingTargetTreeFileHierarchyConflict(candidate, occupied) {
				return false, nil
			}
		}
	}
	return true, nil
}

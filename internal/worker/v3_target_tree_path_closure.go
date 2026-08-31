package worker

import (
	"fmt"
	"sort"
	"strings"
)

type directCodingTargetTreeOccupation struct {
	FilePaths []string
}

func directCodingTargetTreeOccupationFor(
	stack directCodingProjectStack,
	accepted map[string]struct{},
	authoritativePaths []string,
) directCodingTargetTreeOccupation {
	files := make(
		map[string]struct{}, len(stack.TargetTreeReservedPaths)+len(accepted)+len(authoritativePaths),
	)
	for _, artifactPath := range stack.TargetTreeReservedPaths {
		files[artifactPath] = struct{}{}
	}
	for artifactPath := range accepted {
		files[artifactPath] = struct{}{}
	}
	for _, artifactPath := range authoritativePaths {
		files[artifactPath] = struct{}{}
	}
	filePaths := make([]string, 0, len(files))
	for artifactPath := range files {
		filePaths = append(filePaths, artifactPath)
	}
	sort.Strings(filePaths)
	return directCodingTargetTreeOccupation{FilePaths: filePaths}
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
		normalized, err := requireExactDirectCodingPath(candidate)
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

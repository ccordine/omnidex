package worker

import (
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
)

type directCodingTargetTreeOccupation struct {
	FilePaths []string
	Root      *os.Root
}

func directCodingTargetTreeOccupationFor(
	stack directCodingProjectStack,
	accepted map[string]struct{},
	authoritativePaths []string,
	root *os.Root,
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
	return directCodingTargetTreeOccupation{FilePaths: filePaths, Root: root}
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
		if !directCodingTargetTreeWorkspacePathAvailable(occupation.Root, candidate) {
			return false, nil
		}
	}
	return true, nil
}

// directCodingTargetTreeWorkspacePathAvailable probes only the exact
// mechanically proposed path and its parents. It never inventories or parses
// the repository, so unrelated broken and mixed-language state is irrelevant.
func directCodingTargetTreeWorkspacePathAvailable(root *os.Root, candidate string) bool {
	if root == nil {
		return false
	}
	current := ""
	parts := strings.Split(candidate, "/")
	for index, name := range parts {
		current = path.Join(current, name)
		info, err := root.Lstat(current)
		if os.IsNotExist(err) {
			return true
		}
		if err != nil {
			return false
		}
		if index == len(parts)-1 {
			return false
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
	}
	return false
}

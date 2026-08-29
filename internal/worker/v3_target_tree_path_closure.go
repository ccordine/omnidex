package worker

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

var errDirectCodingTargetTreeExistingFileConflict = errors.New(
	"target tree existing-file hierarchy constraint failed",
)

type directCodingTargetTreeOccupation struct {
	FilePaths      []string
	DirectoryPaths []string
}

func directCodingTargetTreeOccupationFor(
	input assemblyline.TargetTreeInput,
	existingPaths []string,
	accepted map[string]struct{},
) directCodingTargetTreeOccupation {
	files := make(
		map[string]struct{}, len(existingPaths)+len(input.ReservedPaths)+len(accepted),
	)
	for _, paths := range [][]string{existingPaths, input.ReservedPaths} {
		for _, artifactPath := range paths {
			files[artifactPath] = struct{}{}
		}
	}
	for artifactPath := range accepted {
		files[artifactPath] = struct{}{}
	}
	filePaths := make([]string, 0, len(files))
	for artifactPath := range files {
		filePaths = append(filePaths, artifactPath)
	}
	sort.Strings(filePaths)
	directories := append([]string(nil), input.ExistingDirs...)
	sort.Strings(directories)
	return directCodingTargetTreeOccupation{
		FilePaths: filePaths, DirectoryPaths: directories,
	}
}

// validateDirectCodingTargetTreePathClosure proves that a target file cannot
// replace a directory, occupy a static/reserved file, or cross a regular-file
// ancestor boundary. An exact current managed file is the sole allowed file
// overlap because that leaf is reconciled in place.
func validateDirectCodingTargetTreePathClosure(
	input assemblyline.TargetTreeInput,
	existingFiles []string,
	target assemblyline.TargetTree,
) error {
	managed := make(map[string]struct{}, len(input.ExistingPaths))
	for _, artifactPath := range input.ExistingPaths {
		managed[artifactPath] = struct{}{}
	}
	reserved := make(map[string]struct{}, len(input.ReservedPaths))
	for _, artifactPath := range input.ReservedPaths {
		reserved[artifactPath] = struct{}{}
	}
	directories := make(map[string]struct{}, len(input.ExistingDirs))
	for _, directory := range input.ExistingDirs {
		directories[directory] = struct{}{}
	}
	for _, candidate := range target.Paths {
		if _, conflict := directories[candidate]; conflict {
			return fmt.Errorf(
				"target-tree file %q occupies existing directory %q", candidate, candidate,
			)
		}
		for _, occupied := range input.ReservedPaths {
			if directCodingTargetTreeFileHierarchyConflict(candidate, occupied) {
				return fmt.Errorf(
					"target-tree file %q crosses reserved file boundary %q", candidate, occupied,
				)
			}
		}
		for _, occupied := range existingFiles {
			if candidate == occupied {
				_, managedLeaf := managed[candidate]
				_, reservedLeaf := reserved[candidate]
				if managedLeaf && !reservedLeaf {
					continue
				}
			}
			if directCodingTargetTreeFileHierarchyConflict(candidate, occupied) {
				return fmt.Errorf(
					"%w: target-tree file %q crosses existing file boundary %q",
					errDirectCodingTargetTreeExistingFileConflict, candidate, occupied,
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

func directCodingTargetTreePairAvailable(
	paths []string,
	occupation directCodingTargetTreeOccupation,
) (bool, error) {
	if len(paths) != 2 || paths[0] == paths[1] {
		return false, fmt.Errorf("mechanical target-tree grammar must return two distinct paths")
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
		if directCodingTargetTreeFileHierarchyConflict(candidate, paths[1-index]) {
			return false, fmt.Errorf("mechanical target-tree paths cross a file hierarchy boundary")
		}
		for _, directory := range occupation.DirectoryPaths {
			if candidate == directory {
				return false, nil
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

package worker

import (
	"errors"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

var errDirectCodingTargetTreeExistingFileConflict = errors.New(
	"target tree existing-file hierarchy constraint failed",
)

type directCodingTargetTreeOccupation struct {
	Root           string
	FilePaths      []string
	DirectoryPaths []string
}

func directCodingTargetTreeOccupationFor(
	root string,
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
	directories := append([]string(nil), input.ExistingDirs...)
	sort.Strings(directories)
	return directCodingTargetTreeOccupation{
		Root: root, FilePaths: filePaths, DirectoryPaths: directories,
	}
}

// validateDirectCodingTargetTreePathClosure proves that a target file cannot
// replace a directory, occupy a static/reserved file, or cross a regular-file
// ancestor boundary. An exact current managed file is the sole allowed file
// overlap because that leaf is reconciled in place.
func validateDirectCodingTargetTreePathClosure(
	root string,
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
		available, err := directCodingTargetTreeWorkspacePathAvailable(root, candidate)
		if err != nil {
			return fmt.Errorf("inspect target-tree file %q: %w", candidate, err)
		}
		if !available {
			return fmt.Errorf(
				"%w: target-tree file %q crosses an existing non-file boundary",
				errDirectCodingTargetTreeExistingFileConflict, candidate,
			)
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
	return directCodingTargetTreePathsAvailable(paths, occupation)
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
		available, err := directCodingTargetTreeWorkspacePathAvailable(
			occupation.Root, candidate,
		)
		if err != nil {
			return false, err
		}
		if !available {
			return false, nil
		}
	}
	return true, nil
}

// directCodingTargetTreeWorkspacePathAvailable inspects only the exact leaf
// being consumed by a code-owned projector. Existing regular files are valid
// reconcile targets. Directories, symlinks, and non-directory ancestors are
// left untouched, and an unrelated workspace node is never a prerequisite.
func directCodingTargetTreeWorkspacePathAvailable(root, candidate string) (bool, error) {
	resolved, err := resolveV3WorkspaceFile(root, candidate)
	if err != nil {
		return false, err
	}
	info, err := os.Lstat(resolved)
	if err == nil {
		return info.Mode().IsRegular(), nil
	}
	if !os.IsNotExist(err) {
		return false, err
	}
	for parent := path.Dir(candidate); parent != "."; parent = path.Dir(parent) {
		resolvedParent, err := resolveV3WorkspaceFile(root, parent)
		if err != nil {
			return false, err
		}
		info, err := os.Lstat(resolvedParent)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return false, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return false, nil
		}
	}
	return true, nil
}

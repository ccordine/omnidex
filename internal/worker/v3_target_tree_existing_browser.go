package worker

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func snapshotDirectCodingTargetTreeOccupation(
	root string,
	stack directCodingProjectStack,
) (directCodingTargetTreeOccupation, error) {
	if stack.ID != genericTypeScriptBrowserAdapter {
		return directCodingTargetTreeOccupation{}, nil
	}
	if root == "" || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return directCodingTargetTreeOccupation{}, fmt.Errorf(
			"target-tree snapshot requires one canonical absolute workspace root",
		)
	}
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return directCodingTargetTreeOccupation{}, fmt.Errorf(
			"target-tree snapshot root is not one exact directory",
		)
	}
	sourceRoot := filepath.Join(root, "src")
	sourceInfo, err := os.Lstat(sourceRoot)
	if os.IsNotExist(err) {
		return directCodingTargetTreeOccupation{}, nil
	}
	if err != nil {
		return directCodingTargetTreeOccupation{}, fmt.Errorf("inspect target-tree source root: %w", err)
	}
	if sourceInfo.Mode()&os.ModeSymlink != 0 {
		return directCodingTargetTreeOccupation{}, fmt.Errorf("target-tree source root src is a symbolic link")
	}
	if !sourceInfo.IsDir() {
		if sourceInfo.Mode().IsRegular() {
			return directCodingTargetTreeOccupation{FilePaths: []string{"src"}}, nil
		}
		return directCodingTargetTreeOccupation{}, fmt.Errorf("target-tree source root src is non-regular")
	}
	occupation := directCodingTargetTreeOccupation{DirectoryPaths: []string{"src"}}
	entries, err := os.ReadDir(sourceRoot)
	if err != nil {
		return directCodingTargetTreeOccupation{}, fmt.Errorf("read target-tree source root: %w", err)
	}
	if len(entries) > 4096 {
		return directCodingTargetTreeOccupation{}, fmt.Errorf("target-tree source root exceeds 4096 entries")
	}
	for _, entry := range entries {
		relative := path.Join("src", entry.Name())
		info, err := os.Lstat(filepath.Join(sourceRoot, entry.Name()))
		if err != nil {
			return directCodingTargetTreeOccupation{}, fmt.Errorf("inspect target-tree path %s: %w", relative, err)
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			return directCodingTargetTreeOccupation{}, fmt.Errorf("target-tree path %s is a symbolic link", relative)
		case info.IsDir():
			occupation.DirectoryPaths = append(occupation.DirectoryPaths, relative)
		case info.Mode().IsRegular():
			occupation.FilePaths = append(occupation.FilePaths, relative)
		default:
			return directCodingTargetTreeOccupation{}, fmt.Errorf("target-tree path %s is non-regular", relative)
		}
	}
	sort.Strings(occupation.FilePaths)
	sort.Strings(occupation.DirectoryPaths)
	return occupation, nil
}

func reconcileTypeScriptBrowserCompleteTargetTree(
	stack directCodingProjectStack,
	authoritativePaths []string,
	current directCodingTargetTreeOccupation,
) (assemblyline.TargetTree, error) {
	if stack.ProjectCompleteTargetTree == nil {
		return assemblyline.TargetTree{}, fmt.Errorf(
			"TypeScript browser stack lacks one code-owned target-tree projector",
		)
	}
	return stack.ProjectCompleteTargetTree(
		directCodingTargetTreeOccupationFor(
			stack, map[string]struct{}{}, authoritativePaths, current,
		),
	)
}

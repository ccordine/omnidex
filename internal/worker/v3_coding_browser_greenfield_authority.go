package worker

import (
	"fmt"
	"os"
	"path/filepath"
)

func validateDirectCodingTypeScriptGreenfieldProgramRoot(
	root string,
	program directCodingProgram,
) error {
	if program.Project.Stack.ID != genericTypeScriptBrowserAdapter {
		return nil
	}
	paths := make([]string, 0, len(program.StaticFiles)+len(program.Source.Documents))
	for _, file := range program.StaticFiles {
		paths = append(paths, file.Path)
	}
	for _, document := range program.Source.Documents {
		paths = append(paths, document.Path)
	}
	if err := requireAbsentDirectCodingUnownedPaths(root, paths); err != nil {
		return fmt.Errorf("TypeScript browser greenfield authority: %w", err)
	}
	return requireAbsentDirectCodingGeneratedHostPaths(root)
}

func validateDirectCodingTypeScriptGreenfieldAssemblyRoot(
	root string,
	assembly directCodingAssembly,
) error {
	paths := make([]string, len(assembly.Files))
	for index, file := range assembly.Files {
		paths[index] = file.Path
	}
	if err := requireAbsentDirectCodingUnownedPaths(root, paths); err != nil {
		return fmt.Errorf("TypeScript browser write authority: %w", err)
	}
	return requireAbsentDirectCodingGeneratedHostPaths(root)
}

func requireAbsentDirectCodingUnownedPaths(root string, paths []string) error {
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 ||
		!filepath.IsAbs(root) || filepath.Clean(root) != root {
		return fmt.Errorf("greenfield authority requires one canonical exact workspace root")
	}
	seen := make(map[string]struct{}, len(paths))
	for _, relative := range paths {
		if _, err := requireExactDirectCodingPath(relative); err != nil {
			return err
		}
		if _, duplicate := seen[relative]; duplicate {
			continue
		}
		seen[relative] = struct{}{}
		if err := requireDirectCodingPathParentsSafe(root, relative); err != nil {
			return err
		}
		_, err := os.Lstat(filepath.Join(root, filepath.FromSlash(relative)))
		switch {
		case os.IsNotExist(err):
			continue
		case err != nil:
			return fmt.Errorf("inspect unowned path %q: %w", relative, err)
		default:
			return fmt.Errorf(
				"path %q already exists without an exact managed-file receipt; arbitrary existing-project mutation is unsupported",
				relative,
			)
		}
	}
	return nil
}

func requireDirectCodingPathParentsSafe(root string, relative string) error {
	parent := filepath.Dir(filepath.FromSlash(relative))
	for parent != "." {
		info, err := os.Lstat(filepath.Join(root, parent))
		switch {
		case os.IsNotExist(err):
			// A missing child does not prove that an existing ancestor is safe.
		case err != nil:
			return fmt.Errorf("inspect parent of unowned path %q: %w", relative, err)
		case info != nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0):
			return fmt.Errorf(
				"parent %q of greenfield path %q is not one exact directory",
				filepath.ToSlash(parent), relative,
			)
		}
		next := filepath.Dir(parent)
		if next == parent {
			return fmt.Errorf("greenfield path %q escaped its workspace parent", relative)
		}
		parent = next
	}
	return nil
}

func requireAbsentDirectCodingGeneratedHostPaths(root string) error {
	present, err := snapshotDirectCodingGeneratedHostPaths(root)
	if err != nil {
		return err
	}
	for _, relative := range directCodingTypeScriptGeneratedHostPaths {
		if present[relative] {
			return fmt.Errorf(
				"generated host path %q already exists; greenfield verification will not mutate unowned tool output",
				relative,
			)
		}
	}
	return nil
}

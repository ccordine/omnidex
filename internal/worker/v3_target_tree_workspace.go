package worker

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
)

// snapshotDirectCodingTargetTreePaths is the code-owned current filesystem
// view given to the structural station. It contains regular workspace files
// only; generated dependency and control directories are not application
// evidence.
func snapshotDirectCodingTargetTreePaths(root string) ([]string, []string, error) {
	paths := make([]string, 0)
	directories := make([]string, 0)
	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".omni", "node_modules", "vendor", "dist", "build", "target":
				if current != root {
					return filepath.SkipDir
				}
			}
			if current == root {
				return nil
			}
			relative, err := filepath.Rel(root, current)
			if err != nil {
				return err
			}
			directories = append(directories, filepath.ToSlash(relative))
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		relative, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		normalized, err := normalizeDirectCodingPath(relative)
		if err != nil || normalized != relative {
			return nil
		}
		paths = append(paths, relative)
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("snapshot target-tree workspace paths: %w", err)
	}
	sort.Strings(paths)
	sort.Strings(directories)
	return paths, directories, nil
}

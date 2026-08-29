package worker

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
		if err := validateV3WritePath(relative); err != nil {
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

func directCodingTargetTreeExistingSource(root, path string) (string, error) {
	target, err := resolveV3WorkspaceFile(root, path)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(target)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read target-tree existing file %s: %w", path, err)
	}
	if len(content) > maxV3WriteBytes {
		return "", fmt.Errorf("target-tree existing file %s exceeds the %d-byte model context limit", path, maxV3WriteBytes)
	}
	if strings.ContainsRune(string(content), '\x00') {
		return "", fmt.Errorf("target-tree existing file %s contains NUL bytes", path)
	}
	return string(content), nil
}

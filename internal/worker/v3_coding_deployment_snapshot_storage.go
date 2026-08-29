package worker

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func directCodingEnsureDeploymentSnapshotBoundary(root string) (string, error) {
	current := root
	for _, name := range []string{".omni", directCodingDeploymentSnapshotDirectory} {
		current = filepath.Join(current, name)
		if err := os.Mkdir(current, 0o700); err != nil && !os.IsExist(err) {
			return "", fmt.Errorf("create deployment snapshot boundary: %w", err)
		}
		info, err := os.Lstat(current)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o700 {
			return "", fmt.Errorf("deployment snapshot boundary is not one exact mode-0700 directory")
		}
	}
	return current, nil
}

func directCodingWriteDeploymentSnapshot(root string, assembly directCodingAssembly) error {
	directories := make(map[string]struct{})
	for _, file := range assembly.Files {
		for parent := filepath.Dir(filepath.FromSlash(file.Path)); parent != "."; parent = filepath.Dir(parent) {
			directories[parent] = struct{}{}
		}
	}
	ordered := make([]string, 0, len(directories))
	for directory := range directories {
		ordered = append(ordered, directory)
	}
	sort.Slice(ordered, func(left, right int) bool {
		leftDepth := strings.Count(ordered[left], string(filepath.Separator))
		rightDepth := strings.Count(ordered[right], string(filepath.Separator))
		return leftDepth < rightDepth || leftDepth == rightDepth && ordered[left] < ordered[right]
	})
	for _, directory := range ordered {
		if err := os.Mkdir(filepath.Join(root, directory), 0o700); err != nil {
			return fmt.Errorf("create deployment snapshot directory %q: %w", filepath.ToSlash(directory), err)
		}
	}
	for _, file := range assembly.Files {
		target := filepath.Join(root, filepath.FromSlash(file.Path))
		handle, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return fmt.Errorf("create deployment snapshot file %q: %w", file.Path, err)
		}
		writeErr := error(nil)
		if _, err := handle.WriteString(file.Content); err != nil {
			writeErr = err
		} else if err := handle.Sync(); err != nil {
			writeErr = err
		} else if err := handle.Chmod(0o444); err != nil {
			writeErr = err
		}
		if closeErr := handle.Close(); writeErr == nil {
			writeErr = closeErr
		}
		if writeErr != nil {
			return fmt.Errorf("seal deployment snapshot file %q: %w", file.Path, writeErr)
		}
	}
	sort.Slice(ordered, func(left, right int) bool {
		return strings.Count(ordered[left], string(filepath.Separator)) >
			strings.Count(ordered[right], string(filepath.Separator))
	})
	for _, directory := range ordered {
		path := filepath.Join(root, directory)
		if err := os.Chmod(path, 0o555); err != nil {
			return fmt.Errorf("seal deployment snapshot directory %q: %w", filepath.ToSlash(directory), err)
		}
		if err := syncDirectCodingDeploymentDirectory(path); err != nil {
			return err
		}
	}
	if err := os.Chmod(root, 0o555); err != nil {
		return fmt.Errorf("seal deployment snapshot root: %w", err)
	}
	return syncDirectCodingDeploymentDirectory(root)
}

func syncDirectCodingDeploymentDirectory(path string) error {
	handle, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open deployment snapshot directory: %w", err)
	}
	syncErr := handle.Sync()
	closeErr := handle.Close()
	if syncErr != nil {
		return fmt.Errorf("sync deployment snapshot directory: %w", syncErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close deployment snapshot directory: %w", closeErr)
	}
	return nil
}

func directCodingRemoveSnapshotStaging(root string) error {
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, _ error) error {
		if entry != nil {
			_ = os.Chmod(path, 0o700)
		}
		return nil
	})
	if err := os.RemoveAll(root); err != nil {
		return fmt.Errorf("clean deployment snapshot staging directory: %w", err)
	}
	return nil
}

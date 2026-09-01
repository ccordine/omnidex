package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func verifyHostAccessMountBoundary(
	root string,
	expected os.FileInfo,
) (mountID uint64, resultErr error) {
	parent := filepath.Dir(root)
	if parent == root {
		return 0, fmt.Errorf("configured root has no distinct canonical parent mount")
	}
	directory, err := os.Open(root)
	if err != nil {
		return 0, fmt.Errorf("open configured root mount: %w", err)
	}
	defer func() {
		resultErr = errors.Join(resultErr, directory.Close())
	}()
	opened, err := directory.Stat()
	if err != nil || !opened.IsDir() || !os.SameFile(expected, opened) {
		return 0, fmt.Errorf("configured root changed during mount attestation")
	}
	parentDirectory, err := os.Open(parent)
	if err != nil {
		return 0, fmt.Errorf("open configured root parent mount %q: %w", parent, err)
	}
	defer func() {
		resultErr = errors.Join(resultErr, parentDirectory.Close())
	}()
	parentInfo, err := parentDirectory.Stat()
	if err != nil || !parentInfo.IsDir() {
		return 0, fmt.Errorf("configured root parent %q is not an accessible directory", parent)
	}
	rootMountID, err := workspaceMountIDForHandle(directory)
	if err != nil {
		return 0, fmt.Errorf("resolve configured root mount identity: %w", err)
	}
	parentMountID, err := workspaceMountIDForHandle(parentDirectory)
	if err != nil {
		return 0, fmt.Errorf("resolve configured root parent mount identity: %w", err)
	}
	if rootMountID == parentMountID {
		return 0, fmt.Errorf("configured root is not an actual mount boundary")
	}
	return rootMountID, nil
}

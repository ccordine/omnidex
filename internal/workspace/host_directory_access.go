package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const maxHostDirectoryAccessPathBytes = 4096

// HostDirectoryAccess declares one immutable allowed path boundary. Filesystem
// consumers validate its current directory and the requested workspace together;
// constructing an unused capability performs no filesystem work.
type HostDirectoryAccess struct {
	root string
}

func NewHostDirectoryAccess(root string) HostDirectoryAccess {
	return HostDirectoryAccess{root: root}
}

// ValidateWorkspaceRoot proves that exactRoot currently resolves to an exact
// directory at or below the configured host boundary. The resolved path is
// deliberately discarded so exactRoot remains the sole project identity.
func (access HostDirectoryAccess) ValidateWorkspaceRoot(exactRoot string) error {
	_, err := access.captureWorkspaceRoot(exactRoot)
	return err
}

func (access HostDirectoryAccess) captureWorkspaceRoot(exactRoot string) (os.FileInfo, error) {
	if err := validateHostAccessPath(access.root, "HOST_DIRECTORY_ACCESS_ROOT"); err != nil {
		return nil, err
	}
	if err := validateHostAccessPath(exactRoot, "client_cwd"); err != nil {
		return nil, err
	}
	relative, err := hostAccessRelativePath(access.root, exactRoot)
	if err != nil {
		return nil, err
	}
	currentRoot, err := exactHostAccessDirectory(access.root, "HOST_DIRECTORY_ACCESS_ROOT")
	if err != nil {
		return nil, err
	}
	resolvedBoundary, err := filepath.EvalSymlinks(access.root)
	if err != nil {
		return nil, fmt.Errorf("resolve HOST_DIRECTORY_ACCESS_ROOT %q: %w", access.root, err)
	}
	if resolvedBoundary != access.root {
		return nil, fmt.Errorf("HOST_DIRECTORY_ACCESS_ROOT %q resolves to %q; configure the exact directory path", access.root, resolvedBoundary)
	}
	requestedInfo, err := exactHostAccessDirectory(exactRoot, "client_cwd")
	if err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(exactRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve client_cwd %q: %w", exactRoot, err)
	}
	if resolved != exactRoot {
		return nil, fmt.Errorf(
			"client_cwd %q resolves to %q; submit the exact physical directory",
			exactRoot,
			resolved,
		)
	}
	if _, err := hostAccessRelativePath(access.root, resolved); err != nil {
		return nil, fmt.Errorf("resolved client_cwd %q: %w", resolved, err)
	}
	if err := verifyOpenedHostAccessDirectory(access.root, relative, currentRoot, requestedInfo); err != nil {
		return nil, fmt.Errorf("verify client_cwd %q within HOST_DIRECTORY_ACCESS_ROOT: %w", exactRoot, err)
	}
	return requestedInfo, nil
}

func validateHostAccessPath(value, label string) error {
	if value == "" || len(value) > maxHostDirectoryAccessPathBytes || !utf8.ValidString(value) ||
		strings.IndexByte(value, 0) >= 0 || value != strings.TrimSpace(value) {
		return fmt.Errorf("%s must contain one exact nonblank path of at most %d bytes", label, maxHostDirectoryAccessPathBytes)
	}
	if !filepath.IsAbs(value) || filepath.Clean(value) != value {
		return fmt.Errorf("%s %q must be one canonical absolute path", label, value)
	}
	return nil
}

func exactHostAccessDirectory(value, label string) (os.FileInfo, error) {
	info, err := os.Lstat(value)
	if err != nil {
		return nil, fmt.Errorf("inspect %s %q: %w", label, value, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s %q is not one exact directory", label, value)
	}
	return info, nil
}

func hostAccessRelativePath(root, candidate string) (string, error) {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return "", fmt.Errorf("relate client_cwd %q to HOST_DIRECTORY_ACCESS_ROOT %q: %w", candidate, root, err)
	}
	parentPrefix := ".." + string(filepath.Separator)
	if relative == ".." || strings.HasPrefix(relative, parentPrefix) || filepath.IsAbs(relative) {
		return "", fmt.Errorf(
			"client_cwd %q is outside HOST_DIRECTORY_ACCESS_ROOT %q",
			candidate,
			root,
		)
	}
	return relative, nil
}

func verifyOpenedHostAccessDirectory(root, relative string, expectedRoot, expected os.FileInfo) (resultErr error) {
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer func() {
		resultErr = errors.Join(resultErr, rootFS.Close())
	}()
	openedRoot, err := rootFS.Stat(".")
	if err != nil {
		return err
	}
	if !openedRoot.IsDir() || !os.SameFile(expectedRoot, openedRoot) {
		return fmt.Errorf("opened host boundary differs from its observed directory")
	}
	directory, err := rootFS.Open(relative)
	if err != nil {
		return err
	}
	defer func() {
		resultErr = errors.Join(resultErr, directory.Close())
	}()
	opened, err := directory.Stat()
	if err != nil {
		return err
	}
	if !opened.IsDir() || !os.SameFile(expected, opened) {
		return fmt.Errorf("opened directory differs from the exact requested path")
	}
	return nil
}

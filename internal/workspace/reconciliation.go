package workspace

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const (
	MaxReconciliationFiles      = 4096
	MaxReconciliationFileBytes  = 32 * 1024 * 1024
	MaxReconciliationTotalBytes = 256 * 1024 * 1024
)

// DesiredFile is the complete requested truth for one workspace path. An
// empty Content slice is an empty file. MoveFrom is a rename optimization;
// Content and Mode remain the authoritative destination state. PreserveExisting
// keeps any existing regular file and uses Content and Mode only when creating
// a missing file.
type DesiredFile struct {
	Path             string
	Present          bool
	Content          []byte
	Mode             uint32
	MoveFrom         string
	PreserveExisting bool
}

type ChangeKind string

const (
	ChangeCreate  ChangeKind = "create"
	ChangeReplace ChangeKind = "replace"
	ChangeDelete  ChangeKind = "delete"
	ChangeMove    ChangeKind = "move"
)

type Change struct {
	Path       string
	Kind       ChangeKind
	SourcePath string
}

type ReconciliationResult struct {
	Changes []Change
}

// VerifiedChangeObserver receives one bounded change only after that exact
// filesystem mutation has been committed and verified. Returning an error
// stops the reconciliation; the applied change remains present in the result.
type VerifiedChangeObserver func(Change) error

// PreparedReconciliation owns one exact desired-state transaction. Preparation
// validates only the values the filesystem consumer needs; it creates no
// staging directories and performs no mutation.
type PreparedReconciliation struct {
	mu       sync.Mutex
	root     string
	rootInfo os.FileInfo
	desired  []DesiredFile
	applied  bool
}

func PrepareReconciliation(
	ctx context.Context,
	root string,
	desired []DesiredFile,
) (*PreparedReconciliation, error) {
	if ctx == nil {
		return nil, fmt.Errorf("workspace reconciliation requires a context")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("prepare workspace reconciliation: %w", err)
	}
	rootInfo, err := exactRootDirectory(root)
	if err != nil {
		return nil, err
	}
	normalized, err := validateDesiredFiles(desired)
	if err != nil {
		return nil, err
	}
	return &PreparedReconciliation{
		root: root, rootInfo: rootInfo, desired: normalized,
	}, nil
}

func exactRootDirectory(root string) (os.FileInfo, error) {
	if root == "" || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return nil, fmt.Errorf("workspace reconciliation requires one canonical absolute root")
	}
	info, err := os.Lstat(root)
	if err != nil {
		return nil, fmt.Errorf("inspect workspace root %q: %w", root, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("workspace root %q is not an exact directory", root)
	}
	return info, nil
}

func validateDesiredFiles(desired []DesiredFile) ([]DesiredFile, error) {
	if len(desired) > MaxReconciliationFiles {
		return nil, fmt.Errorf("workspace reconciliation exceeds %d desired files", MaxReconciliationFiles)
	}
	normalized := make([]DesiredFile, len(desired))
	targets := make(map[string]int, len(desired))
	sources := make(map[string]int)
	totalBytes := 0
	for index, state := range desired {
		if err := validateRelativePath(state.Path); err != nil {
			return nil, fmt.Errorf("workspace desired path %d: %w", index, err)
		}
		if _, duplicate := targets[state.Path]; duplicate {
			return nil, fmt.Errorf("workspace desired state repeats path %q", state.Path)
		}
		if !state.Present {
			if state.Content != nil || state.Mode != 0 || state.MoveFrom != "" || state.PreserveExisting {
				return nil, fmt.Errorf("absent workspace path %q contains file authority", state.Path)
			}
		} else {
			if state.PreserveExisting && state.MoveFrom != "" {
				return nil, fmt.Errorf("workspace path %q cannot preserve and move existing state", state.Path)
			}
			if state.Mode&^uint32(0o777) != 0 {
				return nil, fmt.Errorf("workspace desired file %q has invalid permission bits", state.Path)
			}
			if len(state.Content) > MaxReconciliationFileBytes {
				return nil, fmt.Errorf("workspace desired file %q exceeds %d bytes", state.Path, MaxReconciliationFileBytes)
			}
			if totalBytes > MaxReconciliationTotalBytes-len(state.Content) {
				return nil, fmt.Errorf("workspace desired content exceeds %d total bytes", MaxReconciliationTotalBytes)
			}
			totalBytes += len(state.Content)
		}
		if state.MoveFrom != "" {
			if err := validateRelativePath(state.MoveFrom); err != nil {
				return nil, fmt.Errorf("workspace move source for %q: %w", state.Path, err)
			}
			if state.MoveFrom == state.Path {
				return nil, fmt.Errorf("workspace move source and destination are both %q", state.Path)
			}
			if _, duplicate := sources[state.MoveFrom]; duplicate {
				return nil, fmt.Errorf("workspace move source %q is repeated", state.MoveFrom)
			}
			sources[state.MoveFrom] = index
		}
		targets[state.Path] = index
		normalized[index] = state
		normalized[index].Content = append([]byte(nil), state.Content...)
	}
	paths := make([]string, 0, len(targets)+len(sources))
	for target := range targets {
		paths = append(paths, target)
	}
	for source := range sources {
		paths = append(paths, source)
	}
	sort.Strings(paths)
	for index := 1; index < len(paths); index++ {
		if paths[index] == paths[index-1] {
			continue
		}
		if strings.HasPrefix(paths[index], paths[index-1]+"/") {
			return nil, fmt.Errorf("workspace reconciliation paths %q and %q overlap", paths[index-1], paths[index])
		}
	}
	sort.Slice(normalized, func(left, right int) bool {
		return normalized[left].Path < normalized[right].Path
	})
	return normalized, nil
}

func validateRelativePath(value string) error {
	if value == "" || value != strings.TrimSpace(value) || strings.Contains(value, "\\") || path.IsAbs(value) || path.Clean(value) != value || value == "." || value == ".." || strings.HasPrefix(value, "../") || strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("path %q must be one exact relative slash path", value)
	}
	for _, name := range strings.Split(value, "/") {
		if name == "" || name == "." || name == ".." || len(name) > 255 {
			return fmt.Errorf("path %q contains an invalid basename", value)
		}
	}
	return nil
}

package assemblyline

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

type TargetTreeTransitionKind string

const (
	TargetTreeEnsureDirectory TargetTreeTransitionKind = "ensure_directory"
	TargetTreeCreate          TargetTreeTransitionKind = "create"
	TargetTreeReconcile       TargetTreeTransitionKind = "reconcile"
)

// TargetTreeTransition is one code-owned leaf job. Directories are derived
// from the path by the filesystem layer; no model ever emits operations.
type TargetTreeTransition struct {
	Kind TargetTreeTransitionKind
	Path string
}

// DiffTargetTree creates work only for returned paths. Omission is explicitly
// non-destructive: an existing path absent from the work tree remains untouched.
func DiffTargetTree(input TargetTreeInput, target TargetTree) ([]TargetTreeTransition, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	if len(target.Paths) == 0 {
		return nil, fmt.Errorf("target tree must contain at least one path")
	}
	if err := ValidateTargetTreeExistingDirectories(input.ExistingDirs, target); err != nil {
		return nil, err
	}
	existing := make(map[string]struct{}, len(input.ExistingPaths))
	for _, value := range input.ExistingPaths {
		existing[value] = struct{}{}
	}
	existingDirectories := make(map[string]struct{}, len(input.ExistingDirs))
	for _, value := range input.ExistingDirs {
		existingDirectories[value] = struct{}{}
	}
	neededDirectories := make(map[string]struct{})
	for _, value := range target.Paths {
		for directory := path.Dir(value); directory != "."; directory = path.Dir(directory) {
			if _, exists := existingDirectories[directory]; !exists {
				neededDirectories[directory] = struct{}{}
			}
		}
	}
	directories := make([]string, 0, len(neededDirectories))
	for directory := range neededDirectories {
		directories = append(directories, directory)
	}
	sort.Slice(directories, func(left, right int) bool {
		leftDepth := strings.Count(directories[left], "/")
		rightDepth := strings.Count(directories[right], "/")
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		return directories[left] < directories[right]
	})
	transitions := make([]TargetTreeTransition, 0, len(directories)+len(target.Paths))
	for _, directory := range directories {
		transitions = append(transitions, TargetTreeTransition{Kind: TargetTreeEnsureDirectory, Path: directory})
	}
	for _, value := range target.Paths {
		kind := TargetTreeCreate
		if _, exists := existing[value]; exists {
			kind = TargetTreeReconcile
		}
		transitions = append(transitions, TargetTreeTransition{Kind: kind, Path: value})
	}
	return transitions, nil
}

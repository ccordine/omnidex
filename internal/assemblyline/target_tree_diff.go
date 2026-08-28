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
	TargetTreeDelete          TargetTreeTransitionKind = "delete"
)

// TargetTreeTransition is one code-owned leaf job. Directories are derived
// from the path by the filesystem layer; no model ever emits operations.
type TargetTreeTransition struct {
	Kind TargetTreeTransitionKind
	Path string
}

// DiffTargetTree compares one complete expected workload tree with current
// managed paths. Omission creates a delete only when code separately grants
// deletion eligibility for that exact existing file.
func DiffTargetTree(
	input TargetTreeInput,
	target TargetTree,
	deletionEligible []string,
) ([]TargetTreeTransition, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	if len(target.Paths) == 0 {
		return nil, fmt.Errorf("target tree must contain at least one path")
	}
	if err := validateTargetTreePaths("desired path", target.Paths); err != nil {
		return nil, err
	}
	transitionConstraints := input.Constraints
	transitionConstraints.ExactPathCount = len(target.Paths)
	if err := ValidateTargetTreeConstraints(transitionConstraints, target); err != nil {
		return nil, err
	}
	if err := ValidateTargetTreeExistingDirectories(input.ExistingDirs, target); err != nil {
		return nil, err
	}
	if err := ValidateTargetTreeReservedPaths(input.ReservedPaths, target); err != nil {
		return nil, err
	}
	if err := validateTargetTreePaths("deletion-eligible path", deletionEligible); err != nil {
		return nil, err
	}
	existing := make(map[string]struct{}, len(input.ExistingPaths))
	for _, value := range input.ExistingPaths {
		existing[value] = struct{}{}
	}
	eligible := make(map[string]struct{}, len(deletionEligible))
	for _, value := range deletionEligible {
		if _, exists := existing[value]; !exists {
			return nil, fmt.Errorf("deletion-eligible path %q is not one current managed file", value)
		}
		eligible[value] = struct{}{}
	}
	existingDirectories := make(map[string]struct{}, len(input.ExistingDirs))
	for _, value := range input.ExistingDirs {
		existingDirectories[value] = struct{}{}
	}
	desired := make(map[string]struct{}, len(target.Paths))
	for _, value := range target.Paths {
		desired[value] = struct{}{}
	}
	deletions := make([]string, 0)
	for value := range eligible {
		if _, retained := desired[value]; !retained {
			deletions = append(deletions, value)
		}
	}
	sort.Strings(deletions)
	neededDirectories := make(map[string]struct{})
	for _, value := range target.Paths {
		for directory := path.Dir(value); directory != "."; directory = path.Dir(directory) {
			if _, fileConflict := existing[directory]; fileConflict {
				if _, removable := eligible[directory]; !removable {
					return nil, fmt.Errorf("target-tree directory %q conflicts with a current managed file outside deletion eligibility", directory)
				}
			}
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
	transitions := make([]TargetTreeTransition, 0, len(deletions)+len(directories)+len(target.Paths))
	for _, value := range deletions {
		transitions = append(transitions, TargetTreeTransition{Kind: TargetTreeDelete, Path: value})
	}
	for _, directory := range directories {
		transitions = append(transitions, TargetTreeTransition{Kind: TargetTreeEnsureDirectory, Path: directory})
	}
	paths := append([]string(nil), target.Paths...)
	sort.Strings(paths)
	for _, value := range paths {
		kind := TargetTreeCreate
		if _, exists := existing[value]; exists {
			kind = TargetTreeReconcile
		}
		transitions = append(transitions, TargetTreeTransition{Kind: kind, Path: value})
	}
	return transitions, nil
}

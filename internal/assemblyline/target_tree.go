package assemblyline

import (
	"fmt"
	"path"
	"strings"
	"unicode/utf8"
)

const (
	maxTargetTreePaths = 128
	// MaxTargetTreePathBytes and MaxTargetTreeDepth are shared by target-tree
	// input validation, raw parsing/rendering, and response-bound derivation.
	MaxTargetTreePathBytes      = 512
	MaxTargetTreeDepth          = 16
	maxTargetTreePathBytes      = MaxTargetTreePathBytes
	maxTargetTreeObjectiveBytes = 64 * 1024
)

type TargetArtifactKind string

const (
	TargetArtifactImplementation TargetArtifactKind = "implementation"
	TargetArtifactVerification   TargetArtifactKind = "verification"
)

// TargetTreeConstraints are code-selected structural facts. They constrain
// model output and code-owned candidates identically; they are not inferred.
type TargetTreeConstraints struct {
	ExactPathCount int  `json:"exact_path_count"`
	RootFilesOnly  bool `json:"root_files_only"`
}

func (constraints TargetTreeConstraints) Validate() error {
	if constraints.ExactPathCount < 1 || constraints.ExactPathCount > maxTargetTreePaths {
		return fmt.Errorf(
			"target tree exact path count must be between 1 and %d",
			maxTargetTreePaths,
		)
	}
	return nil
}

// TargetTreeInput is code-owned context for one complete workload-tree
// question. ExistingPaths is the current managed workload tree. ReservedPaths
// contains code-owned leaves that cannot enter the workload tree. ExistingDirs
// is code-only collision evidence and is not model-visible.
type TargetTreeInput struct {
	Objective        string                `json:"objective"`
	TechnicalContext string                `json:"technical_context"`
	Constraints      TargetTreeConstraints `json:"constraints"`
	ExistingPaths    []string              `json:"existing_paths"`
	ReservedPaths    []string              `json:"reserved_paths"`
	ExistingDirs     []string              `json:"existing_dirs"`
	Correction       *TargetTreeCorrection `json:"correction,omitempty"`
}

type TargetTreeCorrection struct {
	CandidateTree string `json:"candidate_tree,omitempty"`
	Failure       string `json:"failure"`
}

type TargetTree struct {
	StackID          string
	VersionProfileID string
	Paths            []string
}

func (input TargetTreeInput) Validate() error {
	if err := validateTargetTreeText("objective", input.Objective, maxTargetTreeObjectiveBytes); err != nil {
		return err
	}
	if err := ValidatePathFreeModelContext("target tree accepted goals", input.Objective); err != nil {
		return err
	}
	if err := validateTargetTreeText("technical context", input.TechnicalContext, maxTargetTreePathBytes); err != nil {
		return err
	}
	if err := input.Constraints.Validate(); err != nil {
		return err
	}
	if input.ExistingPaths == nil {
		return fmt.Errorf("target tree existing workspace paths must be a non-nil array")
	}
	if input.ReservedPaths == nil {
		return fmt.Errorf("target tree reserved paths must be a non-nil array")
	}
	if input.ExistingDirs == nil {
		return fmt.Errorf("target tree existing workspace directories must be a non-nil array")
	}
	if err := validateTargetTreePaths("existing workspace path", input.ExistingPaths); err != nil {
		return err
	}
	if err := validateTargetTreePaths("reserved path", input.ReservedPaths); err != nil {
		return err
	}
	if err := validateTargetTreePaths("existing workspace directory", input.ExistingDirs); err != nil {
		return err
	}
	if correction := input.Correction; correction != nil {
		if correction.CandidateTree != "" {
			if _, err := ParseTargetTree(correction.CandidateTree); err != nil {
				return fmt.Errorf("target tree correction candidate is not a safe raw tree: %w", err)
			}
		}
		if err := validateTargetTreeText("correction failure", correction.Failure, 1200); err != nil {
			return err
		}
	}
	return nil
}

// ValidateTargetTreeConstraints applies the same code-owned structural facts
// at the decoded-candidate, deterministic projection, and compiler boundaries.
func ValidateTargetTreeConstraints(constraints TargetTreeConstraints, target TargetTree) error {
	if err := constraints.Validate(); err != nil {
		return err
	}
	if len(target.Paths) != constraints.ExactPathCount {
		return fmt.Errorf(
			"target tree requires exactly %d paths", constraints.ExactPathCount,
		)
	}
	if constraints.RootFilesOnly {
		for _, artifactPath := range target.Paths {
			if path.Dir(artifactPath) != "." {
				return fmt.Errorf(
					"target-tree path %q must be a file in the workspace root",
					artifactPath,
				)
			}
		}
	}
	return nil
}

// ValidateTargetTreeExistingDirectories prevents a file leaf from claiming an
// exact path that the workspace snapshot proves is already a directory.
func ValidateTargetTreeExistingDirectories(existingDirs []string, target TargetTree) error {
	directories := make(map[string]struct{}, len(existingDirs))
	for _, directory := range existingDirs {
		directories[directory] = struct{}{}
	}
	for _, artifactPath := range target.Paths {
		if _, conflict := directories[artifactPath]; conflict {
			return fmt.Errorf(
				"target-tree path %q conflicts with an existing workspace directory",
				artifactPath,
			)
		}
	}
	return nil
}

// ValidateTargetTreeReservedPaths is the single collision check used by both
// inferred target-tree inputs and code-owned project-stack validation.
func ValidateTargetTreeReservedPaths(reservedPaths []string, target TargetTree) error {
	reserved := make(map[string]struct{}, len(reservedPaths))
	for _, artifactPath := range reservedPaths {
		reserved[artifactPath] = struct{}{}
	}
	for _, artifactPath := range target.Paths {
		if _, conflict := reserved[artifactPath]; conflict {
			return fmt.Errorf("target-tree path %q is reserved and cannot be returned", artifactPath)
		}
	}
	return nil
}

func validateTargetTreePaths(label string, paths []string) error {
	if len(paths) > maxTargetTreePaths {
		return fmt.Errorf("target tree %s set exceeds %d paths", label, maxTargetTreePaths)
	}
	seen := make(map[string]struct{}, len(paths))
	for index, value := range paths {
		if err := validateTargetTreePath(value); err != nil {
			return fmt.Errorf("target tree %s %d: %w", label, index, err)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("target tree %s %d duplicates %q", label, index, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateTargetTreePath(value string) error {
	if value == "" || len(value) > maxTargetTreePathBytes || value != path.Clean(value) ||
		strings.HasPrefix(value, "/") || value == "." || strings.HasPrefix(value, "../") ||
		strings.Contains(value, "\\") {
		return fmt.Errorf("path must be one normalized relative slash path")
	}
	names := strings.Split(value, "/")
	if len(names) > MaxTargetTreeDepth {
		return fmt.Errorf("path exceeds the target-tree depth limit of %d", MaxTargetTreeDepth)
	}
	for _, name := range names {
		if err := validateTargetTreeBasename(name); err != nil {
			return err
		}
	}
	return nil
}

func validateTargetTreeText(label, value string, maximum int) error {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) ||
		strings.ContainsRune(value, '\x00') || len(value) > maximum {
		return fmt.Errorf("target tree %s must be trimmed UTF-8 text of at most %d bytes", label, maximum)
	}
	return nil
}

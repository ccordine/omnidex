package assemblyline

import (
	"errors"
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
)

var (
	errTargetTreeFileCount         = errors.New("target tree file-count constraint failed")
	errTargetTreeRootFilesOnly     = errors.New("target tree root-files-only constraint failed")
	errTargetTreeReservedConflict  = errors.New("target tree reserved-node constraint failed")
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

type TargetTree struct {
	StackID          string
	VersionProfileID string
	Paths            []string
}

// ValidateTargetTreeConstraints applies the same code-owned structural facts
// at the decoded-candidate, deterministic projection, and compiler boundaries.
func ValidateTargetTreeConstraints(constraints TargetTreeConstraints, target TargetTree) error {
	if err := constraints.Validate(); err != nil {
		return err
	}
	if len(target.Paths) != constraints.ExactPathCount {
		return fmt.Errorf(
			"%w: requires exactly %d paths", errTargetTreeFileCount,
			constraints.ExactPathCount,
		)
	}
	if constraints.RootFilesOnly {
		for _, artifactPath := range target.Paths {
			if path.Dir(artifactPath) != "." {
				return fmt.Errorf(
					"%w: target-tree path %q must be a file in the workspace root",
					errTargetTreeRootFilesOnly,
					artifactPath,
				)
			}
		}
	}
	return nil
}

// ValidateTargetTreeReservedPaths is the single collision check used by both
// inferred target-tree inputs and code-owned project-stack validation.
func ValidateTargetTreeReservedPaths(reservedPaths []string, target TargetTree) error {
	for _, artifactPath := range target.Paths {
		for _, reservedPath := range reservedPaths {
			if artifactPath == reservedPath || strings.HasPrefix(artifactPath, reservedPath+"/") ||
				strings.HasPrefix(reservedPath, artifactPath+"/") {
				return fmt.Errorf(
					"%w: target-tree path %q crosses reserved file boundary %q",
					errTargetTreeReservedConflict, artifactPath, reservedPath,
				)
			}
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

func validateTargetTreeBasename(value string) error {
	if value == "" || value == "." || value == ".." || len(value) > 255 ||
		!utf8.ValidString(value) || strings.ContainsAny(value, "/\\\x00\r\n") {
		return fmt.Errorf("path contains an invalid basename")
	}
	return nil
}

func validateTargetTreeText(label, value string, maximum int) error {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) ||
		strings.ContainsRune(value, '\x00') || len(value) > maximum {
		return fmt.Errorf("%s must be trimmed UTF-8 text of at most %d bytes", label, maximum)
	}
	return nil
}

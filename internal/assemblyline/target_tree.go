package assemblyline

import (
	"fmt"
	"path"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	TargetTreeCandidateSchemaV1 = "omnidex.target-tree.v1"
	maxTargetTreePaths          = 128
	maxTargetTreePathBytes      = 512
	maxTargetTreeObjectiveBytes = 4096
)

type TargetArtifactKind string

const (
	TargetArtifactImplementation TargetArtifactKind = "implementation"
	TargetArtifactVerification   TargetArtifactKind = "verification"
)

// TargetTreeInput is code-owned context for one structural question. ExistingPaths
// contains the current workspace plus path leaves already reserved by earlier
// focused tree calls; those paths are evidence, not part of the model's output.
type TargetTreeInput struct {
	Objective        string                `json:"objective"`
	TechnicalContext string                `json:"technical_context"`
	ExistingPaths    []string              `json:"existing_paths"`
	ExistingDirs     []string              `json:"existing_dirs"`
	Correction       *TargetTreeCorrection `json:"correction,omitempty"`
}

type TargetTreeCorrection struct {
	CandidateJSON string `json:"candidate_json"`
	Failure       string `json:"failure"`
}

// TargetTreeCandidate is the whole model boundary: a path-only work tree.
// Path meaning, content, ownership, and operations are intentionally absent.
type TargetTreeCandidate struct {
	Schema string   `json:"schema"`
	Paths  []string `json:"paths"`
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
	if err := validateTargetTreeText("technical context", input.TechnicalContext, maxTargetTreePathBytes); err != nil {
		return err
	}
	if input.ExistingPaths == nil {
		return fmt.Errorf("target tree existing workspace paths must be a non-nil array")
	}
	if input.ExistingDirs == nil {
		return fmt.Errorf("target tree existing workspace directories must be a non-nil array")
	}
	if err := validateTargetTreePaths("existing workspace path", input.ExistingPaths); err != nil {
		return err
	}
	if err := validateTargetTreePaths("existing workspace directory", input.ExistingDirs); err != nil {
		return err
	}
	if correction := input.Correction; correction != nil {
		if err := validateTargetTreeText("correction candidate", correction.CandidateJSON, maxPortableCandidateBytes); err != nil {
			return err
		}
		if err := validateTargetTreeText("correction failure", correction.Failure, 1200); err != nil {
			return err
		}
	}
	return nil
}

func (candidate TargetTreeCandidate) ValidateFor(input TargetTreeInput) (TargetTree, error) {
	var zero TargetTree
	if err := input.Validate(); err != nil {
		return zero, err
	}
	if candidate.Schema != TargetTreeCandidateSchemaV1 {
		return zero, fmt.Errorf("target tree schema must be %q", TargetTreeCandidateSchemaV1)
	}
	if len(candidate.Paths) == 0 || len(candidate.Paths) > maxTargetTreePaths {
		return zero, fmt.Errorf("target tree requires between 1 and %d paths", maxTargetTreePaths)
	}
	if err := validateTargetTreePaths("work path", candidate.Paths); err != nil {
		return zero, err
	}
	paths := append([]string(nil), candidate.Paths...)
	sort.Strings(paths)
	return TargetTree{Paths: paths}, nil
}

func validateTargetTreePaths(label string, paths []string) error {
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
	return nil
}

func validateTargetTreeText(label, value string, maximum int) error {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) ||
		strings.ContainsRune(value, '\x00') || len(value) > maximum {
		return fmt.Errorf("target tree %s must be trimmed UTF-8 text of at most %d bytes", label, maximum)
	}
	return nil
}

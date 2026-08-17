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
	maxTargetTreeArtifacts      = 128
	maxTargetTreePathBytes      = 512
	maxTargetTreePurposeBytes   = 512
	maxTargetTreeRequirements   = 16
)

type TargetArtifactKind string

const (
	TargetArtifactImplementation TargetArtifactKind = "implementation"
	TargetArtifactVerification   TargetArtifactKind = "verification"
)

type TargetTreeRequirement struct {
	ID        string `json:"id"`
	Statement string `json:"statement"`
}

type CurrentTargetArtifact struct {
	ID             string             `json:"id"`
	Path           string             `json:"path"`
	Kind           TargetArtifactKind `json:"kind"`
	Purpose        string             `json:"purpose"`
	RequirementIDs []string           `json:"requirement_ids"`
}

type TargetTreeInput struct {
	Objective    string                  `json:"objective"`
	Requirements []TargetTreeRequirement `json:"requirements"`
	Current      []CurrentTargetArtifact `json:"current"`
}

type TargetTreeCandidate struct {
	Schema    string               `json:"schema"`
	Artifacts []TargetTreeArtifact `json:"artifacts"`
}

// TargetTreeArtifact declares desired structure only. ExistingArtifactID is an
// opaque code-issued identity; NewKey is model-local identity for exactly one
// new file. The model never describes an operation.
type TargetTreeArtifact struct {
	Path               string             `json:"path"`
	Kind               TargetArtifactKind `json:"kind"`
	Purpose            string             `json:"purpose"`
	RequirementIDs     []string           `json:"requirement_ids"`
	ExistingArtifactID string             `json:"existing_artifact_id,omitempty"`
	NewKey             string             `json:"new_key,omitempty"`
}

type TargetTree struct {
	Artifacts []ResolvedTargetArtifact
}

type ResolvedTargetArtifact struct {
	ID             string
	Path           string
	Kind           TargetArtifactKind
	Purpose        string
	RequirementIDs []string
	Existing       bool
}

func (input TargetTreeInput) Validate() error {
	if err := validateTargetTreeText("objective", input.Objective, maxTargetTreePurposeBytes); err != nil {
		return err
	}
	if len(input.Requirements) == 0 || len(input.Requirements) > maxTargetTreeRequirements {
		return fmt.Errorf("target tree requires between 1 and %d accepted requirements", maxTargetTreeRequirements)
	}
	requirementIDs := make(map[string]struct{}, len(input.Requirements))
	for index, requirement := range input.Requirements {
		if err := validateTargetTreeID("requirement ID", requirement.ID); err != nil {
			return fmt.Errorf("target tree requirement %d: %w", index, err)
		}
		if err := validateTargetTreeText("requirement statement", requirement.Statement, maxTargetTreePurposeBytes); err != nil {
			return fmt.Errorf("target tree requirement %d: %w", index, err)
		}
		if _, duplicate := requirementIDs[requirement.ID]; duplicate {
			return fmt.Errorf("target tree requirement %d duplicates ID %q", index, requirement.ID)
		}
		requirementIDs[requirement.ID] = struct{}{}
	}
	if input.Current == nil {
		return fmt.Errorf("target tree current artifact inventory must be a non-nil array")
	}
	seenIDs := make(map[string]struct{}, len(input.Current))
	seenPaths := make(map[string]struct{}, len(input.Current))
	for index, artifact := range input.Current {
		if err := artifact.validate(requirementIDs); err != nil {
			return fmt.Errorf("target tree current artifact %d: %w", index, err)
		}
		if _, duplicate := seenIDs[artifact.ID]; duplicate {
			return fmt.Errorf("target tree current artifact %d duplicates ID %q", index, artifact.ID)
		}
		if _, duplicate := seenPaths[artifact.Path]; duplicate {
			return fmt.Errorf("target tree current artifact %d duplicates path %q", index, artifact.Path)
		}
		seenIDs[artifact.ID] = struct{}{}
		seenPaths[artifact.Path] = struct{}{}
	}
	return nil
}

func (artifact CurrentTargetArtifact) validate(requirements map[string]struct{}) error {
	if err := validateTargetTreeID("current artifact ID", artifact.ID); err != nil {
		return err
	}
	if err := validateTargetTreePath(artifact.Path); err != nil {
		return err
	}
	if err := validateTargetArtifactKind(artifact.Kind); err != nil {
		return err
	}
	if err := validateTargetTreeText("current artifact purpose", artifact.Purpose, maxTargetTreePurposeBytes); err != nil {
		return err
	}
	return validateTargetTreeRequirementIDs(artifact.RequirementIDs, requirements)
}

func (candidate TargetTreeCandidate) ValidateFor(input TargetTreeInput) (TargetTree, error) {
	var zero TargetTree
	if err := input.Validate(); err != nil {
		return zero, err
	}
	if candidate.Schema != TargetTreeCandidateSchemaV1 {
		return zero, fmt.Errorf("target tree schema must be %q", TargetTreeCandidateSchemaV1)
	}
	if len(candidate.Artifacts) == 0 || len(candidate.Artifacts) > maxTargetTreeArtifacts {
		return zero, fmt.Errorf("target tree requires between 1 and %d artifacts", maxTargetTreeArtifacts)
	}
	current := make(map[string]CurrentTargetArtifact, len(input.Current))
	for _, artifact := range input.Current {
		current[artifact.ID] = artifact
	}
	requirements := make(map[string]struct{}, len(input.Requirements))
	for _, requirement := range input.Requirements {
		requirements[requirement.ID] = struct{}{}
	}
	seenPaths := make(map[string]struct{}, len(candidate.Artifacts))
	seenExisting := make(map[string]struct{}, len(candidate.Artifacts))
	seenNew := make(map[string]struct{}, len(candidate.Artifacts))
	resolved := make([]ResolvedTargetArtifact, 0, len(candidate.Artifacts))
	for index, artifact := range candidate.Artifacts {
		if err := artifact.validate(requirements); err != nil {
			return zero, fmt.Errorf("target tree artifact %d: %w", index, err)
		}
		if _, duplicate := seenPaths[artifact.Path]; duplicate {
			return zero, fmt.Errorf("target tree artifact %d duplicates path %q", index, artifact.Path)
		}
		seenPaths[artifact.Path] = struct{}{}
		if artifact.ExistingArtifactID != "" {
			currentArtifact, exists := current[artifact.ExistingArtifactID]
			if !exists {
				return zero, fmt.Errorf("target tree artifact %d references unknown current artifact ID %q", index, artifact.ExistingArtifactID)
			}
			if _, duplicate := seenExisting[artifact.ExistingArtifactID]; duplicate {
				return zero, fmt.Errorf("target tree artifact %d duplicates current artifact ID %q", index, artifact.ExistingArtifactID)
			}
			if artifact.Kind != currentArtifact.Kind {
				return zero, fmt.Errorf("target tree artifact %d changes kind of existing artifact %q", index, artifact.ExistingArtifactID)
			}
			seenExisting[artifact.ExistingArtifactID] = struct{}{}
			resolved = append(resolved, ResolvedTargetArtifact{ID: artifact.ExistingArtifactID, Path: artifact.Path, Kind: artifact.Kind, Purpose: artifact.Purpose, RequirementIDs: append([]string(nil), artifact.RequirementIDs...), Existing: true})
			continue
		}
		if _, duplicate := seenNew[artifact.NewKey]; duplicate {
			return zero, fmt.Errorf("target tree artifact %d duplicates new key %q", index, artifact.NewKey)
		}
		seenNew[artifact.NewKey] = struct{}{}
		resolved = append(resolved, ResolvedTargetArtifact{ID: "new:" + artifact.NewKey, Path: artifact.Path, Kind: artifact.Kind, Purpose: artifact.Purpose, RequirementIDs: append([]string(nil), artifact.RequirementIDs...)})
	}
	sort.Slice(resolved, func(left, right int) bool { return resolved[left].Path < resolved[right].Path })
	target := TargetTree{Artifacts: resolved}
	if err := target.validateRequirementCoverage(input.Requirements); err != nil {
		return zero, err
	}
	return target, nil
}

func (artifact TargetTreeArtifact) validate(requirements map[string]struct{}) error {
	if err := validateTargetTreePath(artifact.Path); err != nil {
		return err
	}
	if err := validateTargetArtifactKind(artifact.Kind); err != nil {
		return err
	}
	if err := validateTargetTreeText("purpose", artifact.Purpose, maxTargetTreePurposeBytes); err != nil {
		return err
	}
	if err := validateTargetTreeRequirementIDs(artifact.RequirementIDs, requirements); err != nil {
		return err
	}
	return artifact.validateIdentity()
}

func validateTargetTreeRequirementIDs(requirementIDs []string, requirements map[string]struct{}) error {
	if len(requirementIDs) == 0 || len(requirementIDs) > maxTargetTreeRequirements {
		return fmt.Errorf("requirement IDs must contain between 1 and %d values", maxTargetTreeRequirements)
	}
	seenRequirements := make(map[string]struct{}, len(requirementIDs))
	for _, requirementID := range requirementIDs {
		if _, exists := requirements[requirementID]; !exists {
			return fmt.Errorf("references unknown requirement ID %q", requirementID)
		}
		if _, duplicate := seenRequirements[requirementID]; duplicate {
			return fmt.Errorf("duplicates requirement ID %q", requirementID)
		}
		seenRequirements[requirementID] = struct{}{}
	}
	return nil
}

func (artifact TargetTreeArtifact) validateIdentity() error {
	if (artifact.ExistingArtifactID == "") == (artifact.NewKey == "") {
		return fmt.Errorf("requires exactly one of existing artifact ID or new key")
	}
	if artifact.ExistingArtifactID != "" {
		return validateTargetTreeID("existing artifact ID", artifact.ExistingArtifactID)
	}
	return validateTargetTreeID("new key", artifact.NewKey)
}

func validateTargetArtifactKind(kind TargetArtifactKind) error {
	switch kind {
	case TargetArtifactImplementation, TargetArtifactVerification:
		return nil
	default:
		return fmt.Errorf("artifact kind %q is unsupported", kind)
	}
}

func (target TargetTree) validateRequirementCoverage(requirements []TargetTreeRequirement) error {
	implementation := make(map[string]int, len(requirements))
	verification := make(map[string]int, len(requirements))
	for _, artifact := range target.Artifacts {
		for _, requirementID := range artifact.RequirementIDs {
			switch artifact.Kind {
			case TargetArtifactImplementation:
				implementation[requirementID]++
			case TargetArtifactVerification:
				verification[requirementID]++
			}
		}
	}
	for _, requirement := range requirements {
		if implementation[requirement.ID] != 1 || verification[requirement.ID] != 1 {
			return fmt.Errorf("target tree requires exactly one implementation and one verification artifact for requirement %q", requirement.ID)
		}
	}
	return nil
}

type TargetTreeRequirementFiles struct {
	ImplementationPath string
	VerificationPath   string
}

func (target TargetTree) RequirementFiles(requirementID string) (TargetTreeRequirementFiles, error) {
	var files TargetTreeRequirementFiles
	for _, artifact := range target.Artifacts {
		for _, boundRequirementID := range artifact.RequirementIDs {
			if boundRequirementID != requirementID {
				continue
			}
			switch artifact.Kind {
			case TargetArtifactImplementation:
				files.ImplementationPath = artifact.Path
			case TargetArtifactVerification:
				files.VerificationPath = artifact.Path
			}
		}
	}
	if files.ImplementationPath == "" || files.VerificationPath == "" {
		return TargetTreeRequirementFiles{}, fmt.Errorf("target tree has no complete artifact pair for requirement %q", requirementID)
	}
	return files, nil
}

func validateTargetTreePath(value string) error {
	if value == "" || len(value) > maxTargetTreePathBytes || value != path.Clean(value) ||
		strings.HasPrefix(value, "/") || value == "." || strings.HasPrefix(value, "../") ||
		strings.Contains(value, "\\") {
		return fmt.Errorf("artifact path must be one normalized relative slash path")
	}
	return nil
}

func validateTargetTreeID(label, value string) error {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) ||
		strings.ContainsRune(value, '\x00') || len(value) > 128 {
		return fmt.Errorf("%s must be trimmed UTF-8 text of at most 128 bytes", label)
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

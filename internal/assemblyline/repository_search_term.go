package assemblyline

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	RepositorySearchTermSchemaV2    = "omnidex.repository-search-anchors.v2"
	maxRepositorySearchConceptBytes = 4 * 1024
	maxRepositorySearchTermBytes    = 256
	maxRepositorySearchAnchors      = 3
)

type RepositorySearchTermInput struct {
	UnresolvedConcept string `json:"unresolved_concept"`
}

type RepositorySearchTermDecision struct {
	Schema  string   `json:"schema"`
	Anchors []string `json:"anchors"`
}

func (input RepositorySearchTermInput) validate() error {
	if strings.TrimSpace(input.UnresolvedConcept) == "" {
		return fmt.Errorf("repository search unresolved concept is blank")
	}
	if len(input.UnresolvedConcept) > maxRepositorySearchConceptBytes {
		return fmt.Errorf("repository search unresolved concept exceeds %d bytes", maxRepositorySearchConceptBytes)
	}
	if !utf8.ValidString(input.UnresolvedConcept) {
		return fmt.Errorf("repository search unresolved concept is not valid UTF-8")
	}
	if strings.ContainsRune(input.UnresolvedConcept, '\x00') {
		return fmt.Errorf("repository search unresolved concept contains NUL")
	}
	return ValidatePathFreeModelContext(
		"repository search unresolved concept", input.UnresolvedConcept,
	)
}

func (decision RepositorySearchTermDecision) ValidateFor(input RepositorySearchTermInput) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != RepositorySearchTermSchemaV2 {
		return fmt.Errorf("repository search anchor schema must be %q", RepositorySearchTermSchemaV2)
	}
	if len(decision.Anchors) < 1 || len(decision.Anchors) > maxRepositorySearchAnchors {
		return fmt.Errorf("repository search requires 1-%d anchors", maxRepositorySearchAnchors)
	}
	seen := make(map[string]struct{}, len(decision.Anchors))
	for index, anchor := range decision.Anchors {
		if err := validateRepositorySearchText(
			fmt.Sprintf("anchor %d", index), anchor, maxRepositorySearchTermBytes,
		); err != nil {
			return err
		}
		if err := ValidatePathFreeModelContext("repository search anchor", anchor); err != nil {
			return fmt.Errorf("repository search anchor %d: %w", index, err)
		}
		if _, duplicate := seen[anchor]; duplicate {
			return fmt.Errorf("repository search anchors must be unique")
		}
		seen[anchor] = struct{}{}
	}
	return nil
}

func (decision RepositorySearchTermDecision) ValidatePathFree(
	provenance ArtifactIdentityProvenance,
) error {
	return ValidatePathFreeModelContextWithProvenance(
		"repository search decision", provenance, decision.Anchors...,
	)
}

func validateRepositorySearchText(label, value string, maximum int) error {
	if len(value) == 0 {
		return fmt.Errorf("repository search %s is empty", label)
	}
	if len(value) > maximum {
		return fmt.Errorf("repository search %s exceeds %d bytes", label, maximum)
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("repository search %s is not valid UTF-8", label)
	}
	if strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("repository search %s contains NUL", label)
	}
	if value != strings.TrimSpace(value) {
		return fmt.Errorf("repository search %s must be trimmed", label)
	}
	return nil
}

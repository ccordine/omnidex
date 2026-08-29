package assemblyline

import (
	"fmt"
	"strings"
)

const ContextRelevanceSchemaV1 = "omnidex.context-relevance.v1"

type ContextRelevanceInput struct {
	ExactInstruction     string                      `json:"exact_instruction"`
	CandidateAuthorities []ContextCandidateAuthority `json:"candidate_authorities"`
	MaxSelections        int                         `json:"max_selections"`
	Scope                ContextScope                `json:"scope,omitempty"`
	KnownArtifactPaths   []string                    `json:"known_artifact_paths"`
}

type ContextRelevanceDecision struct {
	Schema                 string   `json:"schema"`
	ReferencedCandidateIDs []string `json:"referenced_candidate_ids"`
}

// contextRelevanceModelCandidate contains exactly the two values needed for
// the relevance uncertainty: an opaque value to return and the candidate text
// whose relevance must be judged. Source classes and integrity hashes remain
// code-owned portable authority.
type contextRelevanceModelCandidate struct {
	CandidateID string `json:"candidate_id"`
	Content     string `json:"content"`
}

func (input ContextRelevanceInput) validate() error {
	if err := input.Scope.Validate(); err != nil {
		return err
	}
	if err := validateContextExactInstruction(input.ExactInstruction); err != nil {
		return err
	}
	if _, err := validateContextArtifactProvenance(
		"context relevance", input.KnownArtifactPaths,
	); err != nil {
		return err
	}
	if err := validateContextCandidateAuthorities(
		"context relevance",
		input.CandidateAuthorities,
		MaxContextCandidateAuthorities,
		MaxContextCandidateProjectionBytes,
	); err != nil {
		return err
	}
	if input.MaxSelections < 1 || input.MaxSelections > MaxContextRelevanceSelections ||
		input.MaxSelections > len(input.CandidateAuthorities) {
		return fmt.Errorf(
			"context relevance max_selections must be 1..%d and fit the candidate set",
			MaxContextRelevanceSelections,
		)
	}
	return nil
}

func (decision ContextRelevanceDecision) ValidateFor(input ContextRelevanceInput) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != ContextRelevanceSchemaV1 {
		return fmt.Errorf("context relevance schema must be %q", ContextRelevanceSchemaV1)
	}
	if decision.ReferencedCandidateIDs == nil {
		return fmt.Errorf("context relevance candidate IDs must be an explicit array")
	}
	provenance, err := validateContextArtifactProvenance(
		"context relevance", input.KnownArtifactPaths,
	)
	if err != nil {
		return err
	}
	if err := validateContextRawModelOutput(
		"context relevance decision", strings.Join(decision.ReferencedCandidateIDs, "\n"), provenance,
	); err != nil {
		return err
	}
	if len(decision.ReferencedCandidateIDs) > input.MaxSelections {
		return fmt.Errorf("context relevance selection exceeds %d IDs", input.MaxSelections)
	}
	available := make(map[string]struct{}, len(input.CandidateAuthorities))
	for _, candidate := range input.CandidateAuthorities {
		available[candidate.CandidateID] = struct{}{}
	}
	seen := make(map[string]struct{}, len(decision.ReferencedCandidateIDs))
	for index, id := range decision.ReferencedCandidateIDs {
		_, exists := available[id]
		if !exists {
			return fmt.Errorf("context relevance reference %d selects unknown candidate ID %q", index, id)
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("context relevance candidate ID %q is duplicated", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

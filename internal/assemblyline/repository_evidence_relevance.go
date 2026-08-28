package assemblyline

import "fmt"

const (
	RepositoryEvidenceRelevanceSchemaV1 = "omnidex.repository-evidence-relevance.v1"
	maxRepositoryRelevanceCandidates    = 8
	maxRepositoryRelevanceSelections    = 2
)

type RepositoryEvidenceCandidate struct {
	EvidenceID string `json:"evidence_id"`
	Text       string `json:"text"`
}

type RepositoryEvidenceRelevanceInput struct {
	ExactRequirement string                        `json:"exact_requirement"`
	Candidates       []RepositoryEvidenceCandidate `json:"candidates"`
	MaxSelections    int                           `json:"max_selections"`
}

type RepositoryEvidenceRelevanceDecision struct {
	Schema      string   `json:"schema"`
	EvidenceIDs []string `json:"evidence_ids"`
}

func (input RepositoryEvidenceRelevanceInput) Validate() error {
	return input.validate()
}

func (input RepositoryEvidenceRelevanceInput) validate() error {
	if err := validateGroundedText("exact requirement", input.ExactRequirement, maxGroundedRequirementBytes, false); err != nil {
		return err
	}
	if len(input.Candidates) < 1 || len(input.Candidates) > maxRepositoryRelevanceCandidates {
		return fmt.Errorf("repository evidence relevance requires 1..%d candidates", maxRepositoryRelevanceCandidates)
	}
	if input.MaxSelections < 1 || input.MaxSelections > maxRepositoryRelevanceSelections ||
		input.MaxSelections > len(input.Candidates) {
		return fmt.Errorf("repository evidence relevance selection bound must be 1..%d and fit its candidates", maxRepositoryRelevanceSelections)
	}
	seenIDs := make(map[string]struct{}, len(input.Candidates))
	seenText := make(map[string]struct{}, len(input.Candidates))
	total := 0
	for index, candidate := range input.Candidates {
		if err := validateGroundedID("evidence ID", candidate.EvidenceID, maxGroundedEvidenceIDBytes); err != nil {
			return fmt.Errorf("repository relevance candidate %d: %w", index, err)
		}
		if _, duplicate := seenIDs[candidate.EvidenceID]; duplicate {
			return fmt.Errorf("repository relevance evidence ID %q is duplicated", candidate.EvidenceID)
		}
		seenIDs[candidate.EvidenceID] = struct{}{}
		if err := validateGroundedText("evidence text", candidate.Text, maxGroundedEvidenceTextBytes, false); err != nil {
			return fmt.Errorf("repository relevance candidate %s: %w", candidate.EvidenceID, err)
		}
		if _, duplicate := seenText[candidate.Text]; duplicate {
			return fmt.Errorf("repository relevance candidate %s duplicates evidence text", candidate.EvidenceID)
		}
		seenText[candidate.Text] = struct{}{}
		total += len(candidate.EvidenceID) + len(candidate.Text)
	}
	if total > maxGroundedEvidenceTotalBytes {
		return fmt.Errorf("repository relevance candidates exceed %d bytes", maxGroundedEvidenceTotalBytes)
	}
	return nil
}

func (decision RepositoryEvidenceRelevanceDecision) ValidateFor(input RepositoryEvidenceRelevanceInput) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != RepositoryEvidenceRelevanceSchemaV1 {
		return fmt.Errorf("repository evidence relevance schema must be %q", RepositoryEvidenceRelevanceSchemaV1)
	}
	if decision.EvidenceIDs == nil {
		return fmt.Errorf("repository evidence relevance IDs must be an explicit array")
	}
	if len(decision.EvidenceIDs) > input.MaxSelections {
		return fmt.Errorf("repository evidence relevance selection exceeds %d evidence IDs", input.MaxSelections)
	}
	available := make(map[string]struct{}, len(input.Candidates))
	for _, candidate := range input.Candidates {
		available[candidate.EvidenceID] = struct{}{}
	}
	seen := make(map[string]struct{}, len(decision.EvidenceIDs))
	for _, id := range decision.EvidenceIDs {
		if _, exists := available[id]; !exists {
			return fmt.Errorf("repository evidence relevance ID %q was not projected", id)
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("repository evidence relevance ID %q is duplicated", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

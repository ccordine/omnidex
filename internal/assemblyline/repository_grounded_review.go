package assemblyline

import (
	"fmt"
)

const (
	RepositoryGroundedReviewSchemaV1       = "omnidex.repository-grounded-review.v1"
	maxRepositoryGroundedReviewDetailBytes = 512
)

type RepositoryGroundedReviewOutcome string
type RepositoryGroundedIssueKind string

const (
	RepositoryGroundedReviewNone  RepositoryGroundedReviewOutcome = "none"
	RepositoryGroundedReviewIssue RepositoryGroundedReviewOutcome = "issue"

	RepositoryGroundedUnsupportedClaim RepositoryGroundedIssueKind = "unsupported_claim"
	RepositoryGroundedContradiction    RepositoryGroundedIssueKind = "contradicted_evidence"
	RepositoryGroundedRequirementGap   RepositoryGroundedIssueKind = "requirement_mismatch"
)

type RepositoryGroundedReviewInput struct {
	RequirementID    string                    `json:"requirement_id"`
	ExactRequirement string                    `json:"exact_requirement"`
	Context          ObjectiveContext          `json:"objective_context"`
	AnswerText       string                    `json:"answer_text"`
	EvidenceIDs      []string                  `json:"evidence_ids"`
	Evidence         []GroundedEvidenceCapsule `json:"evidence"`
}

type RepositoryGroundedReviewDecision struct {
	Schema    string                          `json:"schema"`
	Outcome   RepositoryGroundedReviewOutcome `json:"outcome"`
	IssueKind RepositoryGroundedIssueKind     `json:"issue_kind"`
	Detail    string                          `json:"detail"`
}

func (input RepositoryGroundedReviewInput) Validate() error {
	return input.validate()
}

func (input RepositoryGroundedReviewInput) validate() error {
	if err := validateGroundedID("requirement ID", input.RequirementID, maxGroundedRequirementIDBytes); err != nil {
		return err
	}
	if err := validateGroundedText("exact requirement", input.ExactRequirement, maxGroundedRequirementBytes, false); err != nil {
		return err
	}
	if err := input.Context.Validate(); err != nil {
		return err
	}
	if err := validateGroundedText("answer text", input.AnswerText, maxGroundedAnswerTextBytes, true); err != nil {
		return err
	}
	if input.EvidenceIDs == nil || len(input.EvidenceIDs) < 1 || len(input.EvidenceIDs) > maxRepositoryRelevanceSelections {
		return fmt.Errorf("repository grounded review requires 1..%d explicit cited evidence IDs", maxRepositoryRelevanceSelections)
	}
	if len(input.Evidence) != len(input.EvidenceIDs) {
		return fmt.Errorf("repository grounded review must project exactly its cited evidence")
	}
	byID := make(map[string]GroundedEvidenceCapsule, len(input.Evidence))
	seenText := make(map[string]struct{}, len(input.Evidence))
	total := 0
	for index, evidence := range input.Evidence {
		if err := validateGroundedID("evidence ID", evidence.ID, maxGroundedEvidenceIDBytes); err != nil {
			return fmt.Errorf("repository review evidence %d: %w", index, err)
		}
		if _, duplicate := byID[evidence.ID]; duplicate {
			return fmt.Errorf("repository review evidence ID %q is duplicated", evidence.ID)
		}
		if err := validateGroundedText("evidence text", evidence.Text, maxGroundedEvidenceTextBytes, false); err != nil {
			return fmt.Errorf("repository review evidence %s: %w", evidence.ID, err)
		}
		if _, duplicate := seenText[evidence.Text]; duplicate {
			return fmt.Errorf("repository review evidence %s duplicates evidence text", evidence.ID)
		}
		seenText[evidence.Text] = struct{}{}
		byID[evidence.ID] = evidence
		total += len(evidence.ID) + len(evidence.Text)
	}
	if total > maxRepositoryRelevanceSelections*maxGroundedEvidenceTextBytes+2*maxGroundedEvidenceIDBytes {
		return fmt.Errorf("repository grounded review evidence projection is oversized")
	}
	seen := make(map[string]struct{}, len(input.EvidenceIDs))
	for _, id := range input.EvidenceIDs {
		if _, exists := byID[id]; !exists {
			return fmt.Errorf("repository grounded review cited evidence %q was not projected", id)
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("repository grounded review cited evidence %q is duplicated", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func (decision RepositoryGroundedReviewDecision) ValidateFor(input RepositoryGroundedReviewInput) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != RepositoryGroundedReviewSchemaV1 {
		return fmt.Errorf("repository grounded review schema must be %q", RepositoryGroundedReviewSchemaV1)
	}
	if decision.Outcome == RepositoryGroundedReviewNone {
		if decision.IssueKind != "" || decision.Detail != "" {
			return fmt.Errorf("repository grounded review NONE must contain no issue state")
		}
		return nil
	}
	if decision.Outcome != RepositoryGroundedReviewIssue {
		return fmt.Errorf("repository grounded review outcome %q is unsupported", decision.Outcome)
	}
	switch decision.IssueKind {
	case RepositoryGroundedUnsupportedClaim, RepositoryGroundedContradiction, RepositoryGroundedRequirementGap:
	default:
		return fmt.Errorf("repository grounded review issue kind %q is unsupported", decision.IssueKind)
	}
	return validateRepositoryGroundedReviewDetail(decision.Detail)
}

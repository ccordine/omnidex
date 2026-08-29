package assemblyline

import (
	"fmt"
	"strings"
)

const WebRelevanceSchemaV1 = "omnidex.web-relevance.v1"

type WebRelevanceCandidate struct {
	CandidateID string `json:"candidate_id"`
	Title       string `json:"title"`
	Snippet     string `json:"snippet"`
	Excerpt     string `json:"excerpt"`
}

type WebRelevanceInput struct {
	ExactQuestion string                  `json:"exact_question"`
	Context       ObjectiveContext        `json:"objective_context"`
	Candidates    []WebRelevanceCandidate `json:"candidates"`
	MaxSelections int                     `json:"max_selections"`
}

type WebRelevanceDecision struct {
	Schema       string   `json:"schema"`
	CandidateIDs []string `json:"candidate_ids"`
}

func (input WebRelevanceInput) validate() error {
	if err := validateExactWebQuestion(input.ExactQuestion); err != nil {
		return err
	}
	if err := input.Context.Validate(); err != nil {
		return err
	}
	if len(input.Candidates) < 1 || len(input.Candidates) > maxWebRelevanceCandidates {
		return fmt.Errorf("web relevance requires 1..%d candidates", maxWebRelevanceCandidates)
	}
	if input.MaxSelections < 1 || input.MaxSelections > len(input.Candidates) {
		return fmt.Errorf("web relevance selection bound must fit the candidate set")
	}
	seen := make(map[string]struct{}, len(input.Candidates))
	total := 0
	for index, candidate := range input.Candidates {
		if err := validateWebLine("candidate ID", candidate.CandidateID, maxWebCandidateIDBytes); err != nil {
			return fmt.Errorf("web relevance candidate %d: %w", index, err)
		}
		if _, duplicate := seen[candidate.CandidateID]; duplicate {
			return fmt.Errorf("web relevance candidate ID %q is duplicated", candidate.CandidateID)
		}
		seen[candidate.CandidateID] = struct{}{}
		if err := validateWebText("title", candidate.Title, maxWebCandidateSummaryBytes, false); err != nil {
			return fmt.Errorf("web relevance candidate %s: %w", candidate.CandidateID, err)
		}
		if err := validateWebText("snippet", candidate.Snippet, maxWebCandidateSummaryBytes, false); err != nil {
			return fmt.Errorf("web relevance candidate %s: %w", candidate.CandidateID, err)
		}
		if err := validateWebText("excerpt", candidate.Excerpt, maxWebCandidateSummaryBytes, false); err != nil {
			return fmt.Errorf("web relevance candidate %s: %w", candidate.CandidateID, err)
		}
		if strings.TrimSpace(candidate.Excerpt) == "" {
			return fmt.Errorf("web relevance candidate %q has no excerpt", candidate.CandidateID)
		}
		total += len(candidate.CandidateID) + len(candidate.Title) + len(candidate.Snippet) + len(candidate.Excerpt)
	}
	if total > maxWebRelevanceProjectionBytes {
		return fmt.Errorf("web relevance projection exceeds %d bytes", maxWebRelevanceProjectionBytes)
	}
	return nil
}

func (decision WebRelevanceDecision) ValidateFor(input WebRelevanceInput) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != WebRelevanceSchemaV1 {
		return fmt.Errorf("web relevance schema must be %q", WebRelevanceSchemaV1)
	}
	if decision.CandidateIDs == nil {
		return fmt.Errorf("web relevance candidate IDs must be an explicit array")
	}
	if len(decision.CandidateIDs) > input.MaxSelections {
		return fmt.Errorf("web relevance selection exceeds %d candidate IDs", input.MaxSelections)
	}
	available := make(map[string]struct{}, len(input.Candidates))
	for _, candidate := range input.Candidates {
		available[candidate.CandidateID] = struct{}{}
	}
	seen := make(map[string]struct{}, len(decision.CandidateIDs))
	for _, id := range decision.CandidateIDs {
		if _, exists := available[id]; !exists {
			return fmt.Errorf("web relevance candidate ID %q was not projected", id)
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("web relevance candidate ID %q is duplicated", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

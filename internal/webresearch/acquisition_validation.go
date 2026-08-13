package webresearch

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/websearch"
)

func validateCandidateReport(report websearch.CandidateReport, query string, acquisitionErr error) error {
	if report.Query != query {
		return fmt.Errorf("%w: discovery query %q does not match requested query %q", ErrInvalidAcquisition, report.Query, query)
	}
	if len(report.Diagnostics) == 0 {
		return fmt.Errorf("%w: discovery diagnostics are required", ErrInvalidAcquisition)
	}
	seen := make(map[websearch.CandidateID]struct{}, len(report.Candidates))
	for _, candidate := range report.Candidates {
		if err := websearch.ValidateCandidate(candidate); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidAcquisition, err)
		}
		if _, duplicate := seen[candidate.ID]; duplicate {
			return fmt.Errorf("%w: duplicate candidate %q", ErrInvalidAcquisition, candidate.ID)
		}
		seen[candidate.ID] = struct{}{}
	}
	if acquisitionErr == nil && len(report.Candidates) == 0 {
		return fmt.Errorf("%w: empty discovery succeeded", ErrInvalidAcquisition)
	}
	if errors.Is(acquisitionErr, websearch.ErrNoCandidates) && len(report.Candidates) != 0 {
		return fmt.Errorf("%w: no-candidates failure included candidates", ErrInvalidAcquisition)
	}
	return nil
}

func validateDocumentReport(report websearch.DocumentReport, candidates []websearch.Candidate, acquisitionErr error) error {
	if len(report.Diagnostics) == 0 {
		return fmt.Errorf("%w: fetch diagnostics are required", ErrInvalidAcquisition)
	}
	byCandidate := make(map[websearch.CandidateID]websearch.Candidate, len(candidates))
	for _, candidate := range candidates {
		byCandidate[candidate.ID] = candidate
	}
	seen := make(map[websearch.DocumentID]struct{}, len(report.Documents))
	for _, document := range report.Documents {
		if err := websearch.ValidateDocument(document); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidAcquisition, err)
		}
		candidate, known := byCandidate[document.CandidateID]
		if !known || candidate.URL != document.URL {
			return fmt.Errorf("%w: document %q is not bound to an explicitly fetched candidate", ErrInvalidAcquisition, document.ID)
		}
		if _, duplicate := seen[document.ID]; duplicate {
			return fmt.Errorf("%w: duplicate document %q", ErrInvalidAcquisition, document.ID)
		}
		seen[document.ID] = struct{}{}
	}
	if acquisitionErr == nil && len(report.Documents) == 0 {
		return fmt.Errorf("%w: empty fetch succeeded", ErrInvalidAcquisition)
	}
	if errors.Is(acquisitionErr, websearch.ErrNoDocuments) && len(report.Documents) != 0 {
		return fmt.Errorf("%w: no-documents failure included documents", ErrInvalidAcquisition)
	}
	for index, diagnostic := range report.Diagnostics {
		if diagnostic.CandidateID == "" || strings.TrimSpace(diagnostic.URL) == "" && diagnostic.Outcome != websearch.FetchEmpty {
			return fmt.Errorf("%w: fetch diagnostic %d is incomplete", ErrInvalidAcquisition, index)
		}
		if _, known := byCandidate[diagnostic.CandidateID]; !known {
			return fmt.Errorf("%w: fetch diagnostic %d names unknown candidate", ErrInvalidAcquisition, index)
		}
	}
	return nil
}

func reduceCandidates(candidates []websearch.Candidate, limit int) ([]websearch.Candidate, error) {
	if limit < 1 {
		return nil, fmt.Errorf("%w: candidate reduction limit must be positive", ErrInvalidAcquisition)
	}
	capacity := len(candidates)
	if capacity > limit {
		capacity = limit
	}
	merged := make([]websearch.Candidate, 0, capacity)
	seen := make(map[websearch.CandidateID]string, len(candidates))
	for _, candidate := range candidates {
		if err := websearch.ValidateCandidate(candidate); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidAcquisition, err)
		}
		if firstURL, duplicate := seen[candidate.ID]; duplicate {
			if firstURL != candidate.URL {
				return nil, fmt.Errorf("%w: candidate %q has conflicting URLs", ErrInvalidAcquisition, candidate.ID)
			}
			continue
		}
		seen[candidate.ID] = candidate.URL
		if len(merged) == limit {
			continue
		}
		copy := candidate
		copy.Sources = append([]websearch.CandidateSource{}, candidate.Sources...)
		merged = append(merged, copy)
	}
	return merged, nil
}

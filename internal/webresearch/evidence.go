package webresearch

import (
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/websearch"
)

func evidenceFromDocuments(documents []websearch.Document) []Evidence {
	evidence := make([]Evidence, len(documents))
	for index, document := range documents {
		evidence[index] = Evidence{
			ID: EvidenceID(document.ID), CandidateID: document.CandidateID,
			URL: document.URL, Title: document.Title,
			Snippet: document.Snippet, Content: document.Content,
			ObservedAt: document.ObservedAt,
			Truncated:  document.Truncated,
		}
	}
	return evidence
}

func validateEvidence(evidence []Evidence) error {
	if len(evidence) == 0 {
		return fmt.Errorf("%w: no evidence exists", ErrEvidenceUnavailable)
	}
	ids := make(map[EvidenceID]struct{}, len(evidence))
	candidates := make(map[websearch.CandidateID]struct{}, len(evidence))
	for _, item := range evidence {
		if item.ID == "" || item.CandidateID == "" ||
			item.URL == "" || item.Content == "" || item.ObservedAt.IsZero() ||
			item.ObservedAt.Location() != time.UTC {
			return fmt.Errorf("%w: evidence %q identity or content is invalid", ErrInvalidAcquisition, item.ID)
		}
		if _, duplicate := ids[item.ID]; duplicate {
			return fmt.Errorf("%w: duplicate evidence %q", ErrInvalidAcquisition, item.ID)
		}
		if _, duplicate := candidates[item.CandidateID]; duplicate {
			return fmt.Errorf("%w: duplicate candidate evidence %q", ErrInvalidAcquisition, item.CandidateID)
		}
		ids[item.ID] = struct{}{}
		candidates[item.CandidateID] = struct{}{}
	}
	return nil
}

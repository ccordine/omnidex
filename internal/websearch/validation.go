package websearch

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func ValidateCandidate(candidate Candidate) error {
	if err := validateCandidateBounds(candidate); err != nil {
		return err
	}
	if err := validateFetchedString("candidate title", candidate.Title); err != nil {
		return err
	}
	if err := validateFetchedString("candidate snippet", candidate.Snippet); err != nil {
		return err
	}
	canonical, err := CanonicalizeURL(candidate.URL)
	if err != nil {
		return fmt.Errorf("candidate %q: %w", candidate.ID, err)
	}
	if canonical != candidate.URL || !validReportID(string(candidate.ID), "candidate_", maxConfiguredCandidates) {
		return fmt.Errorf("candidate %q requires a report-local reference and canonical URL", candidate.ID)
	}
	if len(candidate.Sources) == 0 {
		return fmt.Errorf("candidate %q requires at least one discovery source", candidate.ID)
	}
	for index, source := range candidate.Sources {
		if _, ok := providerDefinitionFor(source.Provider); !ok {
			return fmt.Errorf("candidate %q source %d has unsupported provider %q", candidate.ID, index, source.Provider)
		}
		if strings.TrimSpace(source.SearchURL) == "" || source.Rank < 1 {
			return fmt.Errorf("candidate %q source %d is invalid", candidate.ID, index)
		}
	}
	return nil
}

func ValidateDocument(document Document) error {
	if err := validateDocumentReportBounds(
		DocumentReport{Documents: []Document{document}}, 1, 0, maxDocumentBytes,
	); err != nil {
		return err
	}
	if err := validateFetchedString("document title", document.Title); err != nil {
		return err
	}
	if err := validateFetchedString("document snippet", document.Snippet); err != nil {
		return err
	}
	if err := validateFetchedString("document content", document.Content); err != nil {
		return err
	}
	if !validReportID(string(document.ID), "document_", maxConfiguredDocuments) ||
		!validReportID(string(document.CandidateID), "candidate_", maxConfiguredCandidates) ||
		strings.TrimSpace(document.Content) == "" {
		return fmt.Errorf("document %q requires report-local references and content", document.ID)
	}
	if document.ObservedAt.IsZero() || document.ObservedAt.Location() != time.UTC {
		return fmt.Errorf("document %q requires an exact UTC observation time", document.ID)
	}
	canonical, err := CanonicalizeURL(document.URL)
	if err != nil {
		return fmt.Errorf("document %q: %w", document.ID, err)
	}
	if canonical != document.URL {
		return fmt.Errorf("document %q URL is not canonical", document.ID)
	}
	return nil
}

func validReportID(value, prefix string, maximum int) bool {
	ordinal, err := strconv.Atoi(strings.TrimPrefix(value, prefix))
	return err == nil && ordinal > 0 && ordinal <= maximum && value == prefix+strconv.Itoa(ordinal)
}

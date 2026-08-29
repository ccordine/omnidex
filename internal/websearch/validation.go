package websearch

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
	expected, err := CandidateIDForURL(candidate.URL)
	if err != nil {
		return fmt.Errorf("candidate %q: %w", candidate.ID, err)
	}
	if candidate.ID != expected {
		return fmt.Errorf("candidate %q does not match canonical URL", candidate.ID)
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
	if document.CandidateID == "" || strings.TrimSpace(document.Content) == "" {
		return fmt.Errorf("document %q requires candidate identity and content", document.ID)
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
	expectedCandidate := candidateID(document.URL)
	if document.CandidateID != expectedCandidate {
		return fmt.Errorf("document %q candidate identity does not match URL", document.ID)
	}
	digest := sha256.Sum256([]byte(document.Content))
	contentSHA := hex.EncodeToString(digest[:])
	if contentSHA != document.ContentSHA256 {
		return fmt.Errorf("document %q content SHA does not match content", document.ID)
	}
	if document.ID != documentID(document.URL, contentSHA) {
		return fmt.Errorf("document %q identity does not match URL and content", document.ID)
	}
	return nil
}

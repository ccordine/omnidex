package websearch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (service *Service) Fetch(ctx context.Context, request FetchRequest) (DocumentReport, error) {
	if err := service.validateBoundary(ctx); err != nil {
		return DocumentReport{}, err
	}
	candidates, err := service.validateFetchRequest(request)
	if err != nil {
		return DocumentReport{}, err
	}
	report := DocumentReport{
		Documents:   make([]Document, 0, len(candidates)),
		Diagnostics: make([]DocumentDiagnostic, 0, len(candidates)),
	}
	remaining := service.config.TotalDocumentBytes
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return DocumentReport{}, err
		}
		diagnostic := DocumentDiagnostic{CandidateID: candidate.ID, URL: candidate.URL}
		body, fetchErr := service.getDocument(ctx, candidate.URL)
		if fetchErr != nil {
			if errors.Is(fetchErr, ErrUnsafeURL) || errors.Is(fetchErr, ErrDocumentRedirect) ||
				errors.Is(fetchErr, ErrInvalidFetchedText) {
				return service.cloneDocumentReport(report, fetchErr)
			}
			diagnostic.Outcome = FetchFailed
			diagnostic.Failure = truncateUTF8(fetchErr.Error(), maxDiagnosticFailureBytes)
			report.Diagnostics = append(report.Diagnostics, diagnostic)
			if err := ctx.Err(); err != nil {
				return DocumentReport{}, err
			}
			continue
		}
		observedAt := time.Now().UTC().Truncate(time.Microsecond)
		title, snippet, content := extractDocument(body)
		if err := ctx.Err(); err != nil {
			return DocumentReport{}, err
		}
		if err := validateFetchedString("extracted document title", title); err != nil {
			return service.cloneDocumentReport(report, err)
		}
		if err := validateFetchedString("extracted document snippet", snippet); err != nil {
			return service.cloneDocumentReport(report, err)
		}
		if err := validateFetchedString("extracted document text", content); err != nil {
			return service.cloneDocumentReport(report, err)
		}
		if content == "" {
			diagnostic.Outcome = FetchEmpty
			diagnostic.Failure = "document contains no normalized text"
			report.Diagnostics = append(report.Diagnostics, diagnostic)
			continue
		}
		limit := service.config.PerDocumentBytes
		if remaining < limit {
			limit = remaining
		}
		bounded := truncateUTF8(content, limit)
		if bounded == "" {
			return service.cloneDocumentReport(report, fmt.Errorf("%w: total document budget was exhausted", ErrInvalidFetch))
		}
		remaining -= len(bounded)
		if err := validateFetchedString("bounded document text", bounded); err != nil {
			return service.cloneDocumentReport(report, err)
		}
		digest := sha256.Sum256([]byte(bounded))
		contentSHA := hex.EncodeToString(digest[:])
		if strings.TrimSpace(title) == "" {
			title = candidate.Title
		}
		if strings.TrimSpace(snippet) == "" {
			snippet = candidate.Snippet
		}
		title = truncateUTF8(title, maxCandidateTextBytes)
		snippet = truncateUTF8(snippet, maxCandidateTextBytes)
		if err := validateFetchedString("projected document title", title); err != nil {
			return service.cloneDocumentReport(report, err)
		}
		if err := validateFetchedString("projected document snippet", snippet); err != nil {
			return service.cloneDocumentReport(report, err)
		}
		report.Documents = append(report.Documents, Document{
			ID: documentID(candidate.URL, contentSHA), CandidateID: candidate.ID,
			URL: candidate.URL, Title: title, Snippet: snippet,
			Content: bounded, ContentSHA256: contentSHA, ObservedAt: observedAt,
			Truncated: len(bounded) < len(content),
		})
		if err := ctx.Err(); err != nil {
			return DocumentReport{}, err
		}
		diagnostic.Outcome = FetchSucceeded
		report.Diagnostics = append(report.Diagnostics, diagnostic)
	}
	if len(report.Documents) == 0 {
		return service.cloneDocumentReport(report, fmt.Errorf("%w", ErrNoDocuments))
	}
	return service.cloneDocumentReport(report, nil)
}

func (service *Service) validateFetchRequest(request FetchRequest) ([]Candidate, error) {
	if len(request.Candidates) == 0 {
		return nil, fmt.Errorf("%w: candidates are required", ErrInvalidFetch)
	}
	if len(request.CandidateIDs) == 0 {
		return nil, fmt.Errorf("%w: explicit candidate IDs are required", ErrInvalidFetch)
	}
	if len(request.Candidates) > service.config.MaxCandidates {
		return nil, fmt.Errorf("%w: %d candidate definitions exceed max candidates %d", ErrInvalidFetch, len(request.Candidates), service.config.MaxCandidates)
	}
	if len(request.CandidateIDs) > service.config.MaxDocuments {
		return nil, fmt.Errorf("%w: %d candidate IDs exceed max documents %d", ErrInvalidFetch, len(request.CandidateIDs), service.config.MaxDocuments)
	}
	byID := make(map[CandidateID]Candidate, len(request.Candidates))
	for _, candidate := range request.Candidates {
		if err := ValidateCandidate(candidate); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidFetch, err)
		}
		if _, duplicate := byID[candidate.ID]; duplicate {
			return nil, fmt.Errorf("%w: duplicate candidate definition %q", ErrInvalidFetch, candidate.ID)
		}
		copy := candidate
		copy.Sources = append([]CandidateSource{}, candidate.Sources...)
		byID[candidate.ID] = copy
	}
	selected := make([]Candidate, 0, len(request.CandidateIDs))
	seen := make(map[CandidateID]struct{}, len(request.CandidateIDs))
	for _, id := range request.CandidateIDs {
		if len(id) == 0 || len(id) > 128 {
			return nil, fmt.Errorf("%w: candidate ID must contain 1..128 bytes", ErrInvalidFetch)
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, fmt.Errorf("%w: duplicate candidate ID %q", ErrInvalidFetch, id)
		}
		seen[id] = struct{}{}
		candidate, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("%w: unknown candidate ID %q", ErrInvalidFetch, id)
		}
		selected = append(selected, candidate)
	}
	return selected, nil
}

func (service *Service) cloneDocumentReport(report DocumentReport, runErr error) (DocumentReport, error) {
	if err := validateDocumentReportBounds(
		report, service.config.MaxDocuments, service.config.MaxDocuments, service.config.PerDocumentBytes,
	); err != nil {
		return DocumentReport{}, err
	}
	copy := report
	copy.Documents = append([]Document{}, report.Documents...)
	copy.Diagnostics = append([]DocumentDiagnostic{}, report.Diagnostics...)
	return copy, runErr
}

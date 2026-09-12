package websearch

import (
	"context"
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
			return service.cloneDocumentReport(report, err)
		}
		diagnostic := DocumentDiagnostic{CandidateID: candidate.ID, URL: candidate.URL}
		body, fetchErr := service.getDocument(ctx, candidate.URL)
		if fetchErr != nil {
			diagnostic.Outcome = FetchFailed
			diagnostic.Failure = truncateUTF8(fetchErr.Error(), maxDiagnosticFailureBytes)
			report.Diagnostics = append(report.Diagnostics, diagnostic)
			if err := ctx.Err(); err != nil {
				return service.cloneDocumentReport(report, errors.Join(fetchErr, err))
			}
			if errors.Is(fetchErr, ErrUnsafeURL) || errors.Is(fetchErr, ErrDocumentRedirect) ||
				errors.Is(fetchErr, ErrInvalidFetchedText) {
				return service.cloneDocumentReport(report, fetchErr)
			}
			continue
		}
		observedAt := time.Now().UTC().Truncate(time.Microsecond)
		title, snippet, content := extractDocument(body)
		if err := validateFetchedString("extracted document title", title); err != nil {
			return service.failedDocument(report, diagnostic, err)
		}
		if err := validateFetchedString("extracted document snippet", snippet); err != nil {
			return service.failedDocument(report, diagnostic, err)
		}
		if err := validateFetchedString("extracted document text", content); err != nil {
			return service.failedDocument(report, diagnostic, err)
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
			return service.failedDocument(report, diagnostic, fmt.Errorf("%w: total document budget was exhausted", ErrInvalidFetch))
		}
		remaining -= len(bounded)
		if err := validateFetchedString("bounded document text", bounded); err != nil {
			return service.failedDocument(report, diagnostic, err)
		}
		if strings.TrimSpace(title) == "" {
			title = candidate.Title
		}
		if strings.TrimSpace(snippet) == "" {
			snippet = candidate.Snippet
		}
		title = truncateUTF8(title, maxCandidateTextBytes)
		snippet = truncateUTF8(snippet, maxCandidateTextBytes)
		if err := validateFetchedString("projected document title", title); err != nil {
			return service.failedDocument(report, diagnostic, err)
		}
		if err := validateFetchedString("projected document snippet", snippet); err != nil {
			return service.failedDocument(report, diagnostic, err)
		}
		report.Documents = append(report.Documents, Document{
			ID: DocumentID(fmt.Sprintf("document_%d", len(report.Documents)+1)), CandidateID: candidate.ID,
			URL: candidate.URL, Title: title, Snippet: snippet,
			Content: bounded, ObservedAt: observedAt,
			Truncated: len(bounded) < len(content),
		})
		diagnostic.Outcome = FetchSucceeded
		report.Diagnostics = append(report.Diagnostics, diagnostic)
	}
	if err := ctx.Err(); err != nil {
		return service.cloneDocumentReport(report, err)
	}
	if len(report.Documents) == 0 {
		return service.cloneDocumentReport(report, fmt.Errorf("%w", ErrNoDocuments))
	}
	return service.cloneDocumentReport(report, nil)
}

func (service *Service) failedDocument(report DocumentReport, diagnostic DocumentDiagnostic, err error) (DocumentReport, error) {
	diagnostic.Outcome = FetchFailed
	diagnostic.Failure = truncateUTF8(err.Error(), maxDiagnosticFailureBytes)
	report.Diagnostics = append(report.Diagnostics, diagnostic)
	return service.cloneDocumentReport(report, err)
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
		return DocumentReport{}, errors.Join(runErr, err)
	}
	copy := report
	copy.Documents = append([]Document{}, report.Documents...)
	copy.Diagnostics = append([]DocumentDiagnostic{}, report.Diagnostics...)
	return copy, runErr
}

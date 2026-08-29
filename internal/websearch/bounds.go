package websearch

import (
	"fmt"
	"time"
)

const (
	maxProviderCount          = 4
	maxConfiguredCandidates   = 64
	maxConfiguredDocuments    = 32
	maxDocumentBytes          = 1 << 20
	maxTotalDocumentBytes     = 8 << 20
	maxHTTPResponseBytes      = 8 << 20
	maxHTTPTimeout            = 2 * time.Minute
	maxURLBytes               = 4_096
	maxCandidateTextBytes     = 4_096
	maxDiagnosticFailureBytes = 2_048
)

func validateHardConfigBounds(config Config) error {
	if len(config.Providers) > maxProviderCount {
		return boundError("provider count", len(config.Providers), maxProviderCount)
	}
	if config.Timeout > maxHTTPTimeout {
		return fmt.Errorf("%w: timeout exceeds %s", ErrBoundExceeded, maxHTTPTimeout)
	}
	if config.PerDocumentBytes > maxDocumentBytes {
		return boundError("per-document bytes", config.PerDocumentBytes, maxDocumentBytes)
	}
	if config.TotalDocumentBytes > maxTotalDocumentBytes {
		return boundError("total document bytes", config.TotalDocumentBytes, maxTotalDocumentBytes)
	}
	if config.MaxCandidatesPerProvider > maxConfiguredCandidates {
		return boundError("candidates per provider", config.MaxCandidatesPerProvider, maxConfiguredCandidates)
	}
	if config.MaxCandidates > maxConfiguredCandidates {
		return boundError("candidate count", config.MaxCandidates, maxConfiguredCandidates)
	}
	if config.MaxDocuments > maxConfiguredDocuments {
		return boundError("document count", config.MaxDocuments, maxConfiguredDocuments)
	}
	if config.MaxResponseBytes > maxHTTPResponseBytes {
		return boundError64("HTTP response bytes", config.MaxResponseBytes, maxHTTPResponseBytes)
	}
	return nil
}

func validateCandidateBounds(candidate Candidate) error {
	if len(candidate.ID) > 128 || len(candidate.URL) > maxURLBytes ||
		len(candidate.Title) > maxCandidateTextBytes || len(candidate.Snippet) > maxCandidateTextBytes {
		return fmt.Errorf("%w: candidate scalar field", ErrBoundExceeded)
	}
	if len(candidate.Sources) > maxProviderCount {
		return boundError("candidate source count", len(candidate.Sources), maxProviderCount)
	}
	for _, source := range candidate.Sources {
		if len(source.Provider) > 64 || len(source.SearchURL) > maxURLBytes {
			return fmt.Errorf("%w: candidate source scalar field", ErrBoundExceeded)
		}
	}
	return nil
}

func validateCandidateReportBounds(report CandidateReport, candidates, diagnostics int) error {
	if len(report.Query) > 4_096 {
		return boundError("report query bytes", len(report.Query), 4_096)
	}
	if len(report.Candidates) > candidates {
		return boundError("report candidate count", len(report.Candidates), candidates)
	}
	if len(report.Diagnostics) > diagnostics {
		return boundError("report diagnostic count", len(report.Diagnostics), diagnostics)
	}
	for _, candidate := range report.Candidates {
		if err := validateCandidateBounds(candidate); err != nil {
			return err
		}
	}
	for _, diagnostic := range report.Diagnostics {
		if len(diagnostic.Provider) > 64 || len(diagnostic.SearchURL) > maxURLBytes ||
			len(diagnostic.Failure) > maxDiagnosticFailureBytes {
			return fmt.Errorf("%w: provider diagnostic scalar field", ErrBoundExceeded)
		}
	}
	return nil
}

func validateDocumentReportBounds(report DocumentReport, documents, diagnostics, contentBytes int) error {
	if len(report.Documents) > documents {
		return boundError("report document count", len(report.Documents), documents)
	}
	if len(report.Diagnostics) > diagnostics {
		return boundError("document diagnostic count", len(report.Diagnostics), diagnostics)
	}
	for _, document := range report.Documents {
		if len(document.ID) > 128 || len(document.CandidateID) > 128 || len(document.URL) > maxURLBytes ||
			len(document.Title) > maxCandidateTextBytes || len(document.Snippet) > maxCandidateTextBytes ||
			len(document.Content) > contentBytes || len(document.ContentSHA256) > 64 {
			return fmt.Errorf("%w: document scalar field", ErrBoundExceeded)
		}
	}
	for _, diagnostic := range report.Diagnostics {
		if len(diagnostic.CandidateID) > 128 || len(diagnostic.URL) > maxURLBytes ||
			len(diagnostic.Failure) > maxDiagnosticFailureBytes {
			return fmt.Errorf("%w: document diagnostic scalar field", ErrBoundExceeded)
		}
	}
	return nil
}

func boundError(name string, got, maximum int) error {
	return fmt.Errorf("%w: %s %d exceeds %d", ErrBoundExceeded, name, got, maximum)
}

func boundError64(name string, got int64, maximum int) error {
	return fmt.Errorf("%w: %s %d exceeds %d", ErrBoundExceeded, name, got, maximum)
}

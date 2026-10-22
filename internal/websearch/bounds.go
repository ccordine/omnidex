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

	if config.Timeout > maxHTTPTimeout {
		return fmt.Errorf("%w: timeout exceeds %s", ErrBoundExceeded, maxHTTPTimeout)
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

	return nil
}

func validateCandidateBounds(candidate Candidate) error {

	if len(candidate.Sources) > maxProviderCount {
		return boundError("candidate source count", len(candidate.Sources), maxProviderCount)
	}
	return nil
}

func validateCandidateReportBounds(report CandidateReport, candidates, diagnostics int) error {

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
	return nil
}

func validateDocumentReportBounds(report DocumentReport, documents, diagnostics, contentBytes int) error {
	if len(report.Documents) > documents {
		return boundError("report document count", len(report.Documents), documents)
	}
	if len(report.Diagnostics) > diagnostics {
		return boundError("document diagnostic count", len(report.Diagnostics), diagnostics)
	}
	return nil
}

func boundError(name string, got, maximum int) error {
	return fmt.Errorf("%w: %s %d exceeds %d", ErrBoundExceeded, name, got, maximum)
}

func boundError64(name string, got int64, maximum int) error {
	return fmt.Errorf("%w: %s %d exceeds %d", ErrBoundExceeded, name, got, maximum)
}

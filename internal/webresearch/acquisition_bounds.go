package webresearch

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/websearch"
)

const (
	maxAcquisitionCandidates      = 32
	maxAcquisitionProviders       = 4
	maxAcquisitionDocuments       = 32
	maxAcquisitionURLBytes        = 4_096
	maxAcquisitionTextBytes       = 8_192
	maxAcquisitionContentBytes    = 1 << 20
	maxAcquisitionDiagnosticBytes = 2_048
)

func validateAcquisitionQuery(query string) error {
	if query == "" || query != strings.TrimSpace(query) || len(query) > 1_024 {
		return fmt.Errorf("query must be trimmed and contain 1..1024 bytes")
	}
	return nil
}

func validateCandidateSliceBounds(candidates []websearch.Candidate) error {
	if len(candidates) > maxAcquisitionCandidates {
		return fmt.Errorf("candidate count %d exceeds %d", len(candidates), maxAcquisitionCandidates)
	}
	for _, candidate := range candidates {
		if len(candidate.ID) > 128 || len(candidate.URL) > maxAcquisitionURLBytes ||
			len(candidate.Title) > maxAcquisitionTextBytes || len(candidate.Snippet) > maxAcquisitionTextBytes ||
			len(candidate.Sources) > maxAcquisitionProviders {
			return fmt.Errorf("candidate %q exceeds acquisition bounds", candidate.ID)
		}
		for _, source := range candidate.Sources {
			if len(source.Provider) > 64 || len(source.SearchURL) > maxAcquisitionURLBytes {
				return fmt.Errorf("candidate %q source exceeds acquisition bounds", candidate.ID)
			}
		}
	}
	return nil
}

func validateCandidateReportBounds(report websearch.CandidateReport) error {
	if len(report.Query) > 1_024 {
		return fmt.Errorf("query exceeds 1024 bytes")
	}
	if err := validateCandidateSliceBounds(report.Candidates); err != nil {
		return err
	}
	if len(report.Diagnostics) > maxAcquisitionProviders {
		return fmt.Errorf("diagnostic count %d exceeds %d", len(report.Diagnostics), maxAcquisitionProviders)
	}
	for _, diagnostic := range report.Diagnostics {
		if len(diagnostic.Provider) > 64 || len(diagnostic.SearchURL) > maxAcquisitionURLBytes ||
			len(diagnostic.Failure) > maxAcquisitionDiagnosticBytes {
			return fmt.Errorf("provider diagnostic exceeds acquisition bounds")
		}
	}
	return nil
}

func validateDocumentReportBounds(report websearch.DocumentReport) error {
	if len(report.Documents) > maxAcquisitionDocuments || len(report.Diagnostics) > maxAcquisitionDocuments {
		return fmt.Errorf("document report exceeds %d entries", maxAcquisitionDocuments)
	}
	totalContentBytes := 0
	for _, document := range report.Documents {
		if len(document.ID) > 128 || len(document.CandidateID) > 128 ||
			len(document.URL) > maxAcquisitionURLBytes || len(document.Title) > maxAcquisitionTextBytes ||
			len(document.Snippet) > maxAcquisitionTextBytes || len(document.Content) > maxAcquisitionContentBytes ||
			len(document.ContentSHA256) > 64 {
			return fmt.Errorf("document %q exceeds acquisition bounds", document.ID)
		}
		totalContentBytes += len(document.Content)
		if totalContentBytes > 8<<20 {
			return fmt.Errorf("document content exceeds 8388608 total bytes")
		}
	}
	for _, diagnostic := range report.Diagnostics {
		if len(diagnostic.CandidateID) > 128 || len(diagnostic.URL) > maxAcquisitionURLBytes ||
			len(diagnostic.Failure) > maxAcquisitionDiagnosticBytes {
			return fmt.Errorf("document diagnostic exceeds acquisition bounds")
		}
	}
	return nil
}

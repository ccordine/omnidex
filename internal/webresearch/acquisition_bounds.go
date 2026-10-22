package webresearch

import (
	"fmt"
	"strings"
	"unicode/utf8"

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
	if query == "" || query != strings.TrimSpace(query) ||
		!utf8.ValidString(query) || strings.ContainsRune(query, '\x00') {
		return fmt.Errorf("query must be trimmed and contain 1..4096 valid UTF-8 bytes without NUL")
	}
	return nil
}

func validateCandidateSliceBounds(candidates []websearch.Candidate) error {

	return nil
}

func validateCandidateReportBounds(report websearch.CandidateReport) error {

	if err := validateCandidateSliceBounds(report.Candidates); err != nil {
		return err
	}

	return nil
}

func validateDocumentReportBounds(report websearch.DocumentReport) error {

	totalContentBytes := 0
	for _, document := range report.Documents {

		totalContentBytes += len(document.Content)

	}
	return nil
}

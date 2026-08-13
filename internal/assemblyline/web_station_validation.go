package assemblyline

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	maxWebQuestionBytes                 = 4 * 1024
	maxWebAttemptedQueries              = 4
	maxWebQueryBytes                    = 1024
	maxWebSearchTerms                   = 3
	maxWebSearchTermBytes               = 256
	maxWebRelevanceCandidates           = 32
	maxWebCandidateIDBytes              = 128
	maxWebCandidateSummaryBytes         = 2 * 1024
	maxWebRelevanceProjectionBytes      = 8 * 1024
	maxWebGroundedEvidence              = 32
	maxWebEvidenceIDBytes               = 128
	maxWebEvidenceProjectionBytes       = 8 * 1024
	maxWebSynthesisParagraphs           = 4
	maxWebSynthesisParagraphBytes       = 2 * 1024
	maxWebEvidenceIDsPerParagraph       = 4
	maxWebReviewEvidenceProjectionBytes = 8 * 1024
	maxWebReviewIssueDetailBytes        = 512
	maxWebSynthesisCorrectionBytes      = 12 * 1024
)

func validateExactWebQuestion(question string) error {
	if strings.TrimSpace(question) == "" {
		return fmt.Errorf("web station exact question is blank")
	}
	return validateWebText("exact question", question, maxWebQuestionBytes, false)
}

func validateWebLine(label, value string, maximum int) error {
	if value == "" || value != strings.TrimSpace(value) || strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("web station %s must be one non-empty trimmed line", label)
	}
	return validateWebText(label, value, maximum, true)
}

func validateWebText(label, value string, maximum int, requireNonblank bool) error {
	if requireNonblank && strings.TrimSpace(value) == "" {
		return fmt.Errorf("web station %s is blank", label)
	}
	if len(value) > maximum {
		return fmt.Errorf("web station %s exceeds %d bytes", label, maximum)
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("web station %s is not valid UTF-8", label)
	}
	if strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("web station %s contains NUL", label)
	}
	return nil
}

func decodeWebStationDecision[T any](label, raw string) (T, error) {
	var decision T
	if len(raw) > maxPortableCandidateBytes {
		return decision, fmt.Errorf("%s candidate exceeds %d bytes", label, maxPortableCandidateBytes)
	}
	if err := decodePortablePayload([]byte(raw), &decision); err != nil {
		return decision, fmt.Errorf("decode %s decision: %w", label, err)
	}
	return decision, nil
}

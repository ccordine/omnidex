package assemblyline

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	maxWebQuestionBytes            = 4 * 1024
	maxWebRelevanceCandidates      = 32
	maxWebCandidateIDBytes         = 128
	maxWebCandidateSummaryBytes    = 2 * 1024
	maxWebRelevanceProjectionBytes = 8 * 1024
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

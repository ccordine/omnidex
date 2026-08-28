package assemblyline

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// ValidateResponseCorrectionReplacement accepts only the complete raw
// replacement returned by a response-correction station. The original
// station's decoder remains the sole authority that can bind these bytes to
// typed state; correction never patches or reconstructs that state.
func ValidateResponseCorrectionReplacement(
	input ResponseCorrectionInput,
	replacement string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	if len(replacement) > maxPortableCandidateBytes {
		return "", fmt.Errorf(
			"response correction replacement exceeds %d bytes",
			maxPortableCandidateBytes,
		)
	}
	if !utf8.ValidString(replacement) || strings.ContainsRune(replacement, '\x00') {
		return "", fmt.Errorf("response correction replacement must be valid NUL-free UTF-8")
	}
	if strings.TrimSpace(replacement) == "" {
		return "", fmt.Errorf("response correction replacement is empty")
	}
	if replacement != strings.TrimSpace(replacement) {
		return "", fmt.Errorf("response correction replacement must preserve one exact trimmed leaf")
	}
	return replacement, nil
}

// DecodeResponseCorrectionReplacement restores the immutable correction
// authority from one PortableJob and validates only the raw replacement
// transport. The wrapped station's decoder must validate its semantic type.
func DecodeResponseCorrectionReplacement(
	job PortableJob,
	raw string,
) (string, error) {
	if err := job.Validate(); err != nil {
		return "", err
	}
	if job.Kind != WorkResponseCorrection {
		return "", fmt.Errorf(
			"response correction decoder requires work kind %q, received %q",
			WorkResponseCorrection, job.Kind,
		)
	}
	var input ResponseCorrectionInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return "", err
	}
	return ValidateResponseCorrectionReplacement(input, raw)
}

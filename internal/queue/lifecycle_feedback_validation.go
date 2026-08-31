package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const maxJobFeedbackBytes = 64 * 1024

func validateJobFeedback(feedback string) (string, string, error) {
	return validateLifecycleFeedback(feedback, "job feedback", maxJobFeedbackBytes)
}

func validateReplanFeedback(feedback string) (string, string, error) {
	return validateLifecycleFeedback(
		feedback,
		"replan feedback",
		assemblyline.MaxObjectiveReplanFeedbackBytes,
	)
}

func validateInterruptFeedback(feedback string) (string, string, error) {
	return validateLifecycleFeedback(
		feedback,
		"interrupt feedback",
		assemblyline.MaxObjectiveReplanFeedbackBytes,
	)
}

func validateLifecycleFeedback(
	feedback string,
	subject string,
	maximumBytes int,
) (string, string, error) {
	if !utf8.ValidString(feedback) {
		return "", "", fmt.Errorf("%s must be valid UTF-8", subject)
	}
	if strings.TrimSpace(feedback) == "" {
		return "", "", fmt.Errorf("%s is required", subject)
	}
	if strings.ContainsRune(feedback, '\x00') {
		return "", "", fmt.Errorf("%s must not contain NUL", subject)
	}
	if len(feedback) > maximumBytes {
		return "", "", fmt.Errorf("%s exceeds the %d-byte limit", subject, maximumBytes)
	}
	digest := sha256.Sum256([]byte(feedback))
	return feedback, hex.EncodeToString(digest[:]), nil
}

package queue

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const maxJobFeedbackBytes = 64 * 1024

func validateJobFeedback(feedback string) (string, error) {
	return validateLifecycleFeedback(feedback, "job feedback", maxJobFeedbackBytes)
}

func validateReplanFeedback(feedback string) (string, error) {
	return validateLifecycleFeedback(
		feedback,
		"replan feedback",
		assemblyline.MaxObjectiveControlFeedbackBytes,
	)
}

func validateInterruptFeedback(feedback string) (string, error) {
	return validateLifecycleFeedback(
		feedback,
		"interrupt feedback",
		assemblyline.MaxObjectiveControlFeedbackBytes,
	)
}

func validateSessionReplanFeedback(feedback string) (string, error) {
	return validateLifecycleFeedback(
		feedback,
		"session replan feedback",
		assemblyline.MaxObjectiveReplanFeedbackBytes,
	)
}

func validateLifecycleFeedback(
	feedback string,
	subject string,
	maximumBytes int,
) (string, error) {
	if !utf8.ValidString(feedback) {
		return "", fmt.Errorf("%s must be valid UTF-8", subject)
	}
	if strings.TrimSpace(feedback) == "" {
		return "", fmt.Errorf("%s is required", subject)
	}
	if strings.ContainsRune(feedback, '\x00') {
		return "", fmt.Errorf("%s must not contain NUL", subject)
	}
	if len(feedback) > maximumBytes {
		return "", fmt.Errorf("%s exceeds the %d-byte limit", subject, maximumBytes)
	}
	return feedback, nil
}

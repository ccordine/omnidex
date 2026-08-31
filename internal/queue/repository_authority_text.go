package queue

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
)

var ErrInvalidJobInstruction = errors.New("invalid job instruction")

func validateJobInstruction(instruction string) error {
	if !utf8.ValidString(instruction) {
		return fmt.Errorf("%w: must be valid UTF-8", ErrInvalidJobInstruction)
	}
	if strings.ContainsRune(instruction, '\x00') {
		return fmt.Errorf("%w: must not contain NUL", ErrInvalidJobInstruction)
	}
	if strings.TrimSpace(instruction) == "" {
		return fmt.Errorf("%w: must contain non-whitespace user authority", ErrInvalidJobInstruction)
	}
	return nil
}

func validateChannelMessageContent(content string) error {
	return model.ValidateChannelMessageContent(content)
}

func validateCancelReason(reason string) (string, error) {
	if !utf8.ValidString(reason) {
		return "", fmt.Errorf("cancel reason must be valid UTF-8")
	}
	if strings.ContainsRune(reason, '\x00') {
		return "", fmt.Errorf("cancel reason must not contain NUL")
	}
	if strings.TrimSpace(reason) == "" {
		return "", fmt.Errorf("cancel reason is required")
	}
	if len(reason) > maxReplanFeedbackBytes {
		return "", fmt.Errorf("cancel reason exceeds the %d-byte limit", maxReplanFeedbackBytes)
	}
	return reason, nil
}

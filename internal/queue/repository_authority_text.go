package queue

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
)

func validateJobInstruction(instruction string) error {
	if !utf8.ValidString(instruction) {
		return fmt.Errorf("job instruction must be valid UTF-8")
	}
	if strings.ContainsRune(instruction, '\x00') {
		return fmt.Errorf("job instruction must not contain NUL")
	}
	if strings.TrimSpace(instruction) == "" {
		return fmt.Errorf("job instruction must contain non-whitespace user authority")
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
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "", fmt.Errorf("cancel reason is required")
	}
	return reason, nil
}

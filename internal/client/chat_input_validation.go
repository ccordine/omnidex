package client

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
)

// ValidateSessionTurnText applies the exact free-form boundary before a
// caller reserves an idempotency identity or dispatches a request.
func ValidateSessionTurnText(exactText string) error {
	return model.ValidateChannelMessage(model.ChannelMessageRoleUser, exactText)
}

func ValidateInterruptFeedback(exactText string) error {
	return validateLifecycleControlText(
		exactText,
		"interrupt feedback",
		maxObjectiveControlBytes,
	)
}

func ValidateReplanFeedback(exactText string) error {
	return validateLifecycleControlText(
		exactText,
		"replan feedback",
		maxObjectiveControlBytes,
	)
}

func ValidateCancelReason(exactText string) error {
	return validateLifecycleControlText(
		exactText,
		"cancel reason",
		maxCancelReasonBytes,
	)
}

func validateLifecycleControlText(
	exactText string,
	name string,
	maximum int,
) error {
	if !utf8.ValidString(exactText) || strings.ContainsRune(exactText, '\x00') {
		return fmt.Errorf("%s must be valid UTF-8 without NUL", name)
	}
	if strings.TrimSpace(exactText) == "" {
		return fmt.Errorf("%s is required", name)
	}
	if len(exactText) > maximum {
		return fmt.Errorf("%s exceeds the %d-byte bound", name, maximum)
	}
	return nil
}

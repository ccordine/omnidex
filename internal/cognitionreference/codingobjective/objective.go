package codingobjective

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	maxObjectiveIdentityBytes = 128
	maxTargetIdentityBytes    = 512
	maxRequirementBytes       = 512
)

func (objective Objective) validate() error {
	if !validBoundedText(objective.ID, maxObjectiveIdentityBytes) ||
		strings.ContainsAny(objective.ID, " \t\r\n") {
		return fmt.Errorf("%w: objective ID is invalid", ErrInvalidObjective)
	}
	if objective.Root == "" || !filepath.IsAbs(objective.Root) ||
		filepath.Clean(objective.Root) != objective.Root {
		return fmt.Errorf("%w: root must be one clean absolute path", ErrInvalidObjective)
	}
	if !validBoundedText(objective.Target, maxTargetIdentityBytes) {
		return fmt.Errorf("%w: target is invalid", ErrInvalidObjective)
	}
	if !validBoundedText(objective.RequirementQuote, maxRequirementBytes) {
		return fmt.Errorf("%w: requirement quote is invalid", ErrInvalidObjective)
	}
	if len(objective.Acceptance) != 1 || objective.Acceptance[0] != AcceptanceGoTestsPass {
		return fmt.Errorf(
			"%w: acceptance must contain exactly %q",
			ErrInvalidObjective, AcceptanceGoTestsPass,
		)
	}
	return nil
}

func validBoundedText(value string, limit int) bool {
	return value != "" && value == strings.TrimSpace(value) && utf8.ValidString(value) &&
		!strings.ContainsRune(value, '\x00') && len([]byte(value)) <= limit
}
